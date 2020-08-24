package provider

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/containous/maesh/pkg/annotations"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// MiddlewareBuilder is capable of building a middleware from service annotations.
type MiddlewareBuilder func(annotations map[string]string) (map[string]*dynamic.Middleware, error)

// PortFinder finds service port mappings.
type PortFinder interface {
	Find(namespace, name string, port int32) (int32, bool)
}

// When multiple Traefik Routers listen to the same entrypoint and have the same Rule, the chosen router is the one
// with the highest priority. There are a few cases where this priority is crucial when building the dynamic configuration:
// - When a TrafficSplit is set on a k8s service, 2 Traefik Routers are created. One for accessing the k8s service
//   endpoints and one for accessing the services endpoints mentioned in the TrafficSplit. They both have the same Rule
//   but we should always prioritize the TrafficSplit. Therefore, TrafficSplit Routers should always have a higher priority.
// - When a TrafficTarget Destination targets pods of a k8s service and a TrafficSplit is set on this service. This
//   creates 2 Traefik Routers. One for the TrafficSplit and one for the TrafficTarget. We should always prioritize
//   TrafficSplits Routers and TrafficSplit Routers should always have a higher priority than TrafficTarget Routers.
const (
	priorityService = iota + 1
	priorityTrafficTargetDirect
	priorityTrafficTargetIndirect
	priorityTrafficSplit
)

// Config holds the Provider configuration.
type Config struct {
	MinHTTPPort        int32
	MaxHTTPPort        int32
	ACL                bool
	DefaultTrafficType string
}

// Provider holds the configuration for generating dynamic configuration from a kubernetes cluster state.
type Provider struct {
	config Config

	tcpStateTable          PortFinder
	udpStateTable          PortFinder
	buildServiceMiddleware MiddlewareBuilder

	logger logrus.FieldLogger
}

// New creates a new Provider.
func New(tcpStateTable, udpStateTable PortFinder, middlewareBuilder MiddlewareBuilder, cfg Config, logger logrus.FieldLogger) *Provider {
	return &Provider{
		config:                 cfg,
		tcpStateTable:          tcpStateTable,
		udpStateTable:          udpStateTable,
		logger:                 logger,
		buildServiceMiddleware: middlewareBuilder,
	}
}

// NewDefaultDynamicConfig creates and returns the minimal working dynamic configuration which should be propagated
// to proxy nodes.
func NewDefaultDynamicConfig() *dynamic.Configuration {
	return &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"readiness": {
					Rule:        "Path(`/ping`)",
					EntryPoints: []string{"readiness"},
					Service:     "readiness",
				},
			},
			Services: map[string]*dynamic.Service{
				"readiness": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						PassHostHeader: getBoolRef(true),
						Servers: []dynamic.Server{
							{
								URL: "http://127.0.0.1:8080",
							},
						},
					},
				},
				blockAllServiceKey: {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						PassHostHeader: getBoolRef(false),
					},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{
				blockAllMiddlewareKey: {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{"255.255.255.255"},
					},
				},
			},
		},
	}
}

// BuildConfig builds a dynamic configuration.
func (p *Provider) BuildConfig(t *topology.Topology) *dynamic.Configuration {
	cfg := NewDefaultDynamicConfig()

	for svcKey, svc := range t.Services {
		if err := p.buildConfigForService(t, cfg, svc); err != nil {
			err = fmt.Errorf("unable to build configuration: %v", err)
			svc.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for Service %q: %v", svcKey, err)
		}
	}

	return cfg
}

// buildConfigForService builds the dynamic configuration for the given service.
func (p *Provider) buildConfigForService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service) error {
	trafficType, err := annotations.GetTrafficType(p.config.DefaultTrafficType, svc.Annotations)
	if err != nil {
		return fmt.Errorf("unable to evaluate traffic-type annotation: %w", err)
	}

	scheme, err := annotations.GetScheme(svc.Annotations)
	if err != nil {
		return fmt.Errorf("unable to evaluate scheme annotation: %w", err)
	}

	var middlewareKeys []string

	// Middlewares are currently supported only for HTTP services.
	if trafficType == annotations.ServiceTypeHTTP {
		middlewareKeys, err = p.buildMiddlewaresForConfigFromService(cfg, svc)
		if err != nil {
			return err
		}
	}

	// When ACL mode is on, all traffic must be forbidden unless explicitly authorized via a TrafficTarget.
	if p.config.ACL {
		p.buildACLConfigRoutersAndServices(t, cfg, svc, scheme, trafficType, middlewareKeys)
	} else {
		err = p.buildConfigRoutersAndServices(t, cfg, svc, scheme, trafficType, middlewareKeys)
		if err != nil {
			return err
		}
	}

	for _, tsKey := range svc.TrafficSplits {
		if err := p.buildServiceAndRoutersForTrafficSplit(t, cfg, tsKey, scheme, trafficType, middlewareKeys); err != nil {
			err = fmt.Errorf("unable to build routers and services : %v", err)
			t.TrafficSplits[tsKey].AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficSplit %q: %v", tsKey, err)

			continue
		}
	}

	return nil
}

func (p *Provider) buildMiddlewaresForConfigFromService(cfg *dynamic.Configuration, svc *topology.Service) ([]string, error) {
	var middlewareKeys []string

	middlewares, err := p.buildServiceMiddleware(svc.Annotations)
	if err != nil {
		return middlewareKeys, fmt.Errorf("unable to build middlewares: %w", err)
	}

	for name, middleware := range middlewares {
		middlewareKey := getMiddlewareKey(svc, name)
		cfg.HTTP.Middlewares[middlewareKey] = middleware

		middlewareKeys = append(middlewareKeys, middlewareKey)
	}

	return middlewareKeys, nil
}

func (p *Provider) buildConfigRoutersAndServices(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, scheme, trafficType string, middlewareKeys []string) error {
	err := p.buildServicesAndRoutersForService(t, cfg, svc, scheme, trafficType, middlewareKeys)
	if err != nil {
		return fmt.Errorf("unable to build routers and services: %w", err)
	}

	return nil
}

func (p *Provider) buildACLConfigRoutersAndServices(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, scheme, trafficType string, middlewareKeys []string) {
	if trafficType == annotations.ServiceTypeHTTP {
		p.buildBlockAllRouters(cfg, svc)
	}

	for _, ttKey := range svc.TrafficTargets {
		if err := p.buildServicesAndRoutersForTrafficTarget(t, cfg, ttKey, scheme, trafficType, middlewareKeys); err != nil {
			err = fmt.Errorf("unable to build routers and services: %v", err)
			t.ServiceTrafficTargets[ttKey].AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficTarget %q: %v", ttKey, err)

			continue
		}
	}
}

func (p *Provider) buildServicesAndRoutersForService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, scheme, trafficType string, middlewares []string) error {
	svcKey := topology.Key{Name: svc.Name, Namespace: svc.Namespace}

	switch trafficType {
	case annotations.ServiceTypeHTTP:
		p.buildServicesAndRoutersForHTTPService(t, cfg, svc, scheme, middlewares, svcKey)

	case annotations.ServiceTypeTCP:
		p.buildServicesAndRoutersForTCPService(t, cfg, svc, svcKey)

	case annotations.ServiceTypeUDP:
		p.buildServicesAndRoutersForUDPService(t, cfg, svc, svcKey)

	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
}

func (p *Provider) buildServicesAndRoutersForHTTPService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, scheme string, middlewares []string, svcKey topology.Key) {
	httpRule := buildHTTPRuleFromService(svc)

	for portID, svcPort := range svc.Ports {
		entrypoint, err := p.buildHTTPEntrypoint(portID)
		if err != nil {
			err = fmt.Errorf("unable to build HTTP entrypoint for port %d: %v", svcPort.Port, err)
			svc.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for Service %q: %v", svcKey, err)

			continue
		}

		key := getServiceRouterKeyFromService(svc, svcPort.Port)

		cfg.HTTP.Services[key] = p.buildHTTPServiceFromService(t, svc, scheme, svcPort)
		cfg.HTTP.Routers[key] = buildHTTPRouter(httpRule, entrypoint, middlewares, key, priorityService)
	}
}

func (p *Provider) buildServicesAndRoutersForTCPService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, svcKey topology.Key) {
	rule := buildTCPRouterRule()

	for _, svcPort := range svc.Ports {
		entrypoint, err := p.buildTCPEntrypoint(svc, svcPort.Port)
		if err != nil {
			err = fmt.Errorf("unable to build TCP entrypoint for port %d: %v", svcPort.Port, err)
			svc.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for Service %q: %v", svcKey, err)

			continue
		}

		key := getServiceRouterKeyFromService(svc, svcPort.Port)

		addTCPService(cfg, key, p.buildTCPServiceFromService(t, svc, svcPort))
		addTCPRouter(cfg, key, buildTCPRouter(rule, entrypoint, key))
	}
}

func (p *Provider) buildServicesAndRoutersForUDPService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, svcKey topology.Key) {
	for _, svcPort := range svc.Ports {
		entrypoint, err := p.buildUDPEntrypoint(svc, svcPort.Port)
		if err != nil {
			err = fmt.Errorf("unable to build UDP entrypoint for port %d: %v", svcPort.Port, err)
			svc.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for Service %q: %v", svcKey, err)

			continue
		}

		key := getServiceRouterKeyFromService(svc, svcPort.Port)

		addUDPService(cfg, key, p.buildUDPServiceFromService(t, svc, svcPort))
		addUDPRouter(cfg, key, buildUDPRouter(entrypoint, key))
	}
}

func (p *Provider) buildServicesAndRoutersForTrafficTarget(t *topology.Topology, cfg *dynamic.Configuration, ttKey topology.ServiceTrafficTargetKey, scheme, trafficType string, middlewares []string) error {
	tt, ok := t.ServiceTrafficTargets[ttKey]
	if !ok {
		return fmt.Errorf("unable to find TrafficTarget %q", ttKey)
	}

	ttSvc, ok := t.Services[tt.Service]
	if !ok {
		return fmt.Errorf("unable to find Service %q", tt.Service)
	}

	switch trafficType {
	case annotations.ServiceTypeHTTP:
		p.buildHTTPServicesAndRoutersForTrafficTarget(t, tt, cfg, ttSvc, ttKey, scheme, middlewares)

	case annotations.ServiceTypeTCP:
		p.buildTCPServicesAndRoutersForTrafficTarget(t, tt, cfg, ttSvc, ttKey)
	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
}

func (p *Provider) buildHTTPServicesAndRoutersForTrafficTarget(t *topology.Topology, tt *topology.ServiceTrafficTarget, cfg *dynamic.Configuration, ttSvc *topology.Service, ttKey topology.ServiceTrafficTargetKey, scheme string, middlewares []string) {
	whitelistDirect := p.buildWhitelistMiddlewareFromTrafficTargetDirect(t, tt)
	whitelistDirectKey := getWhitelistMiddlewareKeyFromTrafficTargetDirect(tt)
	cfg.HTTP.Middlewares[whitelistDirectKey] = whitelistDirect

	rule := buildHTTPRuleFromTrafficTarget(tt, ttSvc)

	for portID, svcPort := range tt.Destination.Ports {
		entrypoint, err := p.buildHTTPEntrypoint(portID)
		if err != nil {
			err = fmt.Errorf("unable to build HTTP entrypoint for port %d: %v", svcPort.Port, err)
			tt.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficTarget %q: %v", ttKey, err)

			continue
		}

		svcKey := getServiceKeyFromTrafficTarget(tt, svcPort.Port)
		cfg.HTTP.Services[svcKey] = p.buildHTTPServiceFromTrafficTarget(t, tt, scheme, svcPort)

		rtrMiddlewares := addToSliceCopy(middlewares, whitelistDirectKey)

		directRtrKey := getRouterKeyFromTrafficTargetDirect(tt, svcPort.Port)
		cfg.HTTP.Routers[directRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewares, svcKey, priorityTrafficTargetDirect)

		// If the ServiceTrafficTarget is the backend of at least one TrafficSplit we need an additional router with
		// a whitelist middleware which whitelists based on the X-Forwarded-For header instead of on the RemoteAddr value.
		if len(ttSvc.BackendOf) > 0 {
			whitelistIndirect := p.buildWhitelistMiddlewareFromTrafficTargetIndirect(t, tt)
			whitelistIndirectKey := getWhitelistMiddlewareKeyFromTrafficTargetIndirect(tt)
			cfg.HTTP.Middlewares[whitelistIndirectKey] = whitelistIndirect

			rule = buildHTTPRuleFromTrafficTargetIndirect(tt, ttSvc)
			rtrMiddlewares = addToSliceCopy(middlewares, whitelistIndirectKey)

			indirectRtrKey := getRouterKeyFromTrafficTargetIndirect(tt, svcPort.Port)
			cfg.HTTP.Routers[indirectRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewares, svcKey, priorityTrafficTargetIndirect)
		}
	}
}

func (p *Provider) buildTCPServicesAndRoutersForTrafficTarget(t *topology.Topology, tt *topology.ServiceTrafficTarget, cfg *dynamic.Configuration, ttSvc *topology.Service, ttKey topology.ServiceTrafficTargetKey) {
	if !hasTrafficTargetRuleTCPRoute(tt) {
		return
	}

	rule := buildTCPRouterRule()

	for _, svcPort := range tt.Destination.Ports {
		entrypoint, err := p.buildTCPEntrypoint(ttSvc, svcPort.Port)
		if err != nil {
			err = fmt.Errorf("unable to build TCP entrypoint for port %d: %v", svcPort.Port, err)
			tt.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficTarget %q: %v", ttKey, err)

			continue
		}

		key := getServiceRouterKeyFromService(ttSvc, svcPort.Port)

		addTCPService(cfg, key, p.buildTCPServiceFromTrafficTarget(t, tt, svcPort))
		addTCPRouter(cfg, key, buildTCPRouter(rule, entrypoint, key))
	}
}

func (p *Provider) buildServiceAndRoutersForTrafficSplit(t *topology.Topology, cfg *dynamic.Configuration, tsKey topology.Key, scheme, trafficType string, middlewares []string) error {
	ts, ok := t.TrafficSplits[tsKey]
	if !ok {
		return fmt.Errorf("unable to find TrafficSplit %q", tsKey)
	}

	tsSvc, ok := t.Services[ts.Service]
	if !ok {
		return fmt.Errorf("unable to find Service %q", ts.Service)
	}

	switch trafficType {
	case annotations.ServiceTypeHTTP:
		p.buildHTTPServiceAndRoutersForTrafficSplit(t, cfg, tsKey, scheme, ts, tsSvc, middlewares)

	case annotations.ServiceTypeTCP:
		p.buildTCPServiceAndRoutersForTrafficSplit(cfg, tsKey, ts, tsSvc)

	case annotations.ServiceTypeUDP:
		p.buildUDPServiceAndRoutersForTrafficSplit(cfg, tsKey, ts, tsSvc)

	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
}

func (p *Provider) buildHTTPServiceAndRoutersForTrafficSplit(t *topology.Topology, cfg *dynamic.Configuration, tsKey topology.Key, scheme string, ts *topology.TrafficSplit, tsSvc *topology.Service, middlewares []string) {
	rule := buildHTTPRuleFromTrafficSplit(ts, tsSvc)

	rtrMiddlewares := middlewares

	if p.config.ACL {
		whitelistDirect := p.buildWhitelistMiddlewareFromTrafficSplitDirect(t, ts)
		whitelistDirectKey := getWhitelistMiddlewareKeyFromTrafficSplitDirect(ts)
		cfg.HTTP.Middlewares[whitelistDirectKey] = whitelistDirect

		rtrMiddlewares = addToSliceCopy(middlewares, whitelistDirectKey)
	}

	for portID, svcPort := range tsSvc.Ports {
		backendSvcs, err := p.buildServicesForTrafficSplitBackends(t, cfg, ts, svcPort, scheme)
		if err != nil {
			err = fmt.Errorf("unable to build HTTP backend services and port %d: %v", svcPort.Port, err)
			ts.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficSplit %q: %v", tsKey, err)

			continue
		}

		entrypoint, err := p.buildHTTPEntrypoint(portID)
		if err != nil {
			err = fmt.Errorf("unable to build HTTP entrypoint for port %d: %v", svcPort.Port, err)
			ts.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficSplit %q: %v", tsKey, err)

			continue
		}

		svcKey := getServiceKeyFromTrafficSplit(ts, svcPort.Port)
		cfg.HTTP.Services[svcKey] = buildHTTPServiceFromTrafficSplit(backendSvcs)

		directRtrKey := getRouterKeyFromTrafficSplitDirect(ts, svcPort.Port)
		cfg.HTTP.Routers[directRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewares, svcKey, priorityTrafficSplit)

		// If the ServiceTrafficSplit is a backend of at least one TrafficSplit we need an additional router with
		// a whitelist middleware which whitelists based on the X-Forwarded-For header instead of on the RemoteAddr value.
		if len(tsSvc.BackendOf) > 0 && p.config.ACL {
			whitelistIndirect := p.buildWhitelistMiddlewareFromTrafficSplitIndirect(t, ts)
			whitelistIndirectKey := getWhitelistMiddlewareKeyFromTrafficSplitIndirect(ts)
			cfg.HTTP.Middlewares[whitelistIndirectKey] = whitelistIndirect

			rule = buildHTTPRuleFromTrafficSplitIndirect(ts, tsSvc)
			rtrMiddlewaresindirect := addToSliceCopy(middlewares, whitelistIndirectKey)

			indirectRtrKey := getRouterKeyFromTrafficSplitIndirect(ts, svcPort.Port)
			cfg.HTTP.Routers[indirectRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewaresindirect, svcKey, priorityTrafficTargetIndirect)
		}
	}
}

func (p *Provider) buildTCPServiceAndRoutersForTrafficSplit(cfg *dynamic.Configuration, tsKey topology.Key, ts *topology.TrafficSplit, tsSvc *topology.Service) {
	tcpRule := buildTCPRouterRule()

	for _, svcPort := range tsSvc.Ports {
		entrypoint, err := p.buildTCPEntrypoint(tsSvc, svcPort.Port)
		if err != nil {
			err = fmt.Errorf("unable to build TCP entrypoint for port %d: %v", svcPort.Port, err)
			ts.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficSplit %q: %v", tsKey, err)

			continue
		}

		backendSvcs := make([]dynamic.TCPWRRService, len(ts.Backends))

		for i, backend := range ts.Backends {
			backendSvcKey := getServiceKeyFromTrafficSplitBackend(ts, svcPort.Port, backend)

			addTCPService(cfg, backendSvcKey, buildTCPSplitTrafficBackendService(backend, svcPort.TargetPort.IntVal))

			backendSvcs[i] = dynamic.TCPWRRService{
				Name:   backendSvcKey,
				Weight: getIntRef(backend.Weight),
			}
		}

		key := getServiceRouterKeyFromService(tsSvc, svcPort.Port)

		addTCPService(cfg, key, buildTCPServiceFromTrafficSplit(backendSvcs))
		addTCPRouter(cfg, key, buildTCPRouter(tcpRule, entrypoint, key))
	}
}

func (p *Provider) buildUDPServiceAndRoutersForTrafficSplit(cfg *dynamic.Configuration, tsKey topology.Key, ts *topology.TrafficSplit, tsSvc *topology.Service) {
	for _, svcPort := range tsSvc.Ports {
		entrypoint, err := p.buildUDPEntrypoint(tsSvc, svcPort.Port)
		if err != nil {
			err = fmt.Errorf("unable to build UDP entrypoint for port %d: %v", svcPort.Port, err)
			ts.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for TrafficSplit %q: %v", tsKey, err)

			continue
		}

		backendSvcs := make([]dynamic.UDPWRRService, len(ts.Backends))

		for i, backend := range ts.Backends {
			backendSvcKey := getServiceKeyFromTrafficSplitBackend(ts, svcPort.Port, backend)

			addUDPService(cfg, backendSvcKey, buildUDPSplitTrafficBackendService(backend, svcPort.TargetPort.IntVal))

			backendSvcs[i] = dynamic.UDPWRRService{
				Name:   backendSvcKey,
				Weight: getIntRef(backend.Weight),
			}
		}

		key := getServiceRouterKeyFromService(tsSvc, svcPort.Port)

		addUDPService(cfg, key, buildUDPServiceFromTrafficSplit(backendSvcs))
		addUDPRouter(cfg, key, buildUDPRouter(entrypoint, key))
	}
}

func (p *Provider) buildServicesForTrafficSplitBackends(t *topology.Topology, cfg *dynamic.Configuration, ts *topology.TrafficSplit, svcPort corev1.ServicePort, scheme string) ([]dynamic.WRRService, error) {
	backendSvcs := make([]dynamic.WRRService, len(ts.Backends))

	for i, backend := range ts.Backends {
		backendSvc, ok := t.Services[backend.Service]
		if !ok {
			return nil, fmt.Errorf("unable to find Service %q", backend.Service)
		}

		if len(backendSvc.TrafficSplits) > 0 {
			tsKey := topology.Key{Name: ts.Name, Namespace: ts.Namespace}
			p.logger.Warnf("Nested TrafficSplits detected in TrafficSplit %q: Maesh doesn't support nested TrafficSplits", tsKey)
		}

		backendSvcKey := getServiceKeyFromTrafficSplitBackend(ts, svcPort.Port, backend)

		cfg.HTTP.Services[backendSvcKey] = buildHTTPSplitTrafficBackendService(backend, scheme, svcPort.Port)
		backendSvcs[i] = dynamic.WRRService{
			Name:   backendSvcKey,
			Weight: getIntRef(backend.Weight),
		}
	}

	return backendSvcs, nil
}

func (p *Provider) buildBlockAllRouters(cfg *dynamic.Configuration, svc *topology.Service) {
	rule := buildHTTPRuleFromService(svc)

	for portID, svcPort := range svc.Ports {
		entrypoint, err := p.buildHTTPEntrypoint(portID)
		if err != nil {
			svcKey := topology.Key{Name: svc.Name, Namespace: svc.Namespace}
			err = fmt.Errorf("unable to build HTTP entrypoint for port %d: %w", svcPort.Port, err)
			svc.AddError(err)
			p.logger.Errorf("Error building dynamic configuration for Service %q: %v", svcKey, err)

			continue
		}

		key := getServiceRouterKeyFromService(svc, svcPort.Port)
		cfg.HTTP.Routers[key] = &dynamic.Router{
			EntryPoints: []string{entrypoint},
			Middlewares: []string{blockAllMiddlewareKey},
			Service:     blockAllServiceKey,
			Rule:        rule,
			Priority:    priorityService,
		}
	}
}

func (p Provider) buildHTTPEntrypoint(portID int) (string, error) {
	port := p.config.MinHTTPPort + int32(portID)
	if port > p.config.MaxHTTPPort {
		return "", errors.New("too many HTTP entrypoints")
	}

	return fmt.Sprintf("http-%d", port), nil
}

func (p Provider) buildTCPEntrypoint(svc *topology.Service, port int32) (string, error) {
	meshPort, ok := p.tcpStateTable.Find(svc.Namespace, svc.Name, port)
	if !ok {
		return "", errors.New("port not found")
	}

	return fmt.Sprintf("tcp-%d", meshPort), nil
}

func (p Provider) buildUDPEntrypoint(svc *topology.Service, port int32) (string, error) {
	meshPort, ok := p.udpStateTable.Find(svc.Namespace, svc.Name, port)
	if !ok {
		return "", errors.New("port not found")
	}

	return fmt.Sprintf("udp-%d", meshPort), nil
}

func (p *Provider) buildHTTPServiceFromService(t *topology.Topology, svc *topology.Service, scheme string, svcPort corev1.ServicePort) *dynamic.Service {
	var servers []dynamic.Server

	for _, podKey := range svc.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q for HTTP service from Service %s@%s", podKey, topology.Key{Name: svc.Name, Namespace: svc.Namespace})
			continue
		}

		hostPort, ok := topology.ResolveServicePort(svcPort, pod.ContainerPorts)
		if !ok {
			p.logger.Warnf("Unable to resolve HTTP service port %q for Pod %q", svcPort.Name, podKey)
			continue
		}

		address := net.JoinHostPort(pod.IP, strconv.Itoa(int(hostPort)))

		servers = append(servers, dynamic.Server{
			URL: fmt.Sprintf("%s://%s", scheme, address),
		})
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers:        servers,
			PassHostHeader: getBoolRef(true),
		},
	}
}

func (p *Provider) buildHTTPServiceFromTrafficTarget(t *topology.Topology, tt *topology.ServiceTrafficTarget, scheme string, svcPort corev1.ServicePort) *dynamic.Service {
	var servers []dynamic.Server

	for _, podKey := range tt.Destination.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q for HTTP service from Traffic Target %q", podKey, topology.ServiceTrafficTargetKey{
				Service:       tt.Service,
				TrafficTarget: topology.Key{Name: tt.Name, Namespace: tt.Namespace},
			})

			continue
		}

		hostPort, ok := topology.ResolveServicePort(svcPort, pod.ContainerPorts)
		if !ok {
			p.logger.Warnf("Unable to resolve HTTP service port %q for Pod %q", svcPort.TargetPort, podKey)
			continue
		}

		address := net.JoinHostPort(pod.IP, strconv.Itoa(int(hostPort)))

		servers = append(servers, dynamic.Server{
			URL: fmt.Sprintf("%s://%s", scheme, address),
		})
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers:        servers,
			PassHostHeader: getBoolRef(true),
		},
	}
}

func (p *Provider) buildTCPServiceFromService(t *topology.Topology, svc *topology.Service, svcPort corev1.ServicePort) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	for _, podKey := range svc.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q for TCP service from Service %s@%s", podKey, topology.Key{Name: svc.Name, Namespace: svc.Namespace})
			continue
		}

		hostPort, ok := topology.ResolveServicePort(svcPort, pod.ContainerPorts)
		if !ok {
			p.logger.Warnf("Unable to resolve TCP service port %q for Pod %q", svcPort.Name, podKey)
			continue
		}

		address := net.JoinHostPort(pod.IP, strconv.Itoa(int(hostPort)))

		servers = append(servers, dynamic.TCPServer{
			Address: address,
		})
	}

	return &dynamic.TCPService{
		LoadBalancer: &dynamic.TCPServersLoadBalancer{
			Servers: servers,
		},
	}
}

func (p *Provider) buildTCPServiceFromTrafficTarget(t *topology.Topology, tt *topology.ServiceTrafficTarget, svcPort corev1.ServicePort) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	for _, podKey := range tt.Destination.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q for TCP service from Traffic Target %s@%s", podKey, topology.Key{Name: tt.Name, Namespace: tt.Namespace})
			continue
		}

		hostPort, ok := topology.ResolveServicePort(svcPort, pod.ContainerPorts)
		if !ok {
			p.logger.Warnf("Unable to resolve TCP service port %q for Pod %q", svcPort.Name, podKey)
			continue
		}

		address := net.JoinHostPort(pod.IP, strconv.Itoa(int(hostPort)))

		servers = append(servers, dynamic.TCPServer{
			Address: address,
		})
	}

	return &dynamic.TCPService{
		LoadBalancer: &dynamic.TCPServersLoadBalancer{
			Servers: servers,
		},
	}
}

func (p *Provider) buildUDPServiceFromService(t *topology.Topology, svc *topology.Service, svcPort corev1.ServicePort) *dynamic.UDPService {
	var servers []dynamic.UDPServer

	for _, podKey := range svc.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q for UDP service from Service %s@%s", podKey, topology.Key{Name: svc.Name, Namespace: svc.Namespace})
			continue
		}

		hostPort, ok := topology.ResolveServicePort(svcPort, pod.ContainerPorts)
		if !ok {
			p.logger.Warnf("Unable to resolve UDP service port %q for Pod %q", svcPort.Name, podKey)
			continue
		}

		address := net.JoinHostPort(pod.IP, strconv.Itoa(int(hostPort)))

		servers = append(servers, dynamic.UDPServer{
			Address: address,
		})
	}

	return &dynamic.UDPService{
		LoadBalancer: &dynamic.UDPServersLoadBalancer{
			Servers: servers,
		},
	}
}

// buildWhitelistMiddlewareFromTrafficTargetDirect builds an IPWhiteList middleware which blocks requests from
// unauthorized Pods. Authorized Pods are those listed in the ServiceTrafficTarget.Sources.
// This middleware doesn't work if used behind a proxy.
func (p *Provider) buildWhitelistMiddlewareFromTrafficTargetDirect(t *topology.Topology, tt *topology.ServiceTrafficTarget) *dynamic.Middleware {
	var IPs []string

	for _, source := range tt.Sources {
		for _, podKey := range source.Pods {
			pod, ok := t.Pods[podKey]
			if !ok {
				p.logger.Errorf("Unable to find Pod %q for WhitelistMiddleware from Traffic Target %s@%s", podKey, topology.Key{Name: tt.Name, Namespace: tt.Namespace})
				continue
			}

			IPs = append(IPs, pod.IP)
		}
	}

	return &dynamic.Middleware{
		IPWhiteList: &dynamic.IPWhiteList{
			SourceRange: IPs,
		},
	}
}

// buildWhitelistMiddlewareFromTrafficSplitDirect builds an IPWhiteList middleware which blocks requests from
// unauthorized Pods. Authorized Pods are those that can access all the leaves of the TrafficSplit.
// This middleware doesn't work if used behind a proxy.
func (p *Provider) buildWhitelistMiddlewareFromTrafficSplitDirect(t *topology.Topology, ts *topology.TrafficSplit) *dynamic.Middleware {
	var IPs []string

	for _, podKey := range ts.Incoming {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q for WhitelistMiddleware from Traffic Split %s@%s", podKey, topology.Key{Name: ts.Name, Namespace: ts.Namespace})
			continue
		}

		IPs = append(IPs, pod.IP)
	}

	return &dynamic.Middleware{
		IPWhiteList: &dynamic.IPWhiteList{
			SourceRange: IPs,
		},
	}
}

// buildWhitelistMiddlewareFromTrafficTargetIndirect builds an IPWhiteList middleware which blocks requests from
// unauthorized Pods. Authorized Pods are those listed in the ServiceTrafficTarget.Sources.
// This middleware works only when used behind a proxy.
func (p *Provider) buildWhitelistMiddlewareFromTrafficTargetIndirect(t *topology.Topology, tt *topology.ServiceTrafficTarget) *dynamic.Middleware {
	whitelist := p.buildWhitelistMiddlewareFromTrafficTargetDirect(t, tt)
	whitelist.IPWhiteList.IPStrategy = &dynamic.IPStrategy{
		Depth: 1,
	}

	return whitelist
}

// buildWhitelistMiddlewareFromTrafficSplitIndirect builds an IPWhiteList middleware which blocks requests from
// unauthorized Pods. Authorized Pods are those that can access all the leaves of the TrafficSplit.
// This middleware works only when used behind a proxy.
func (p *Provider) buildWhitelistMiddlewareFromTrafficSplitIndirect(t *topology.Topology, ts *topology.TrafficSplit) *dynamic.Middleware {
	whitelist := p.buildWhitelistMiddlewareFromTrafficSplitDirect(t, ts)
	whitelist.IPWhiteList.IPStrategy = &dynamic.IPStrategy{
		Depth: 1,
	}

	return whitelist
}

func buildHTTPServiceFromTrafficSplit(backendSvc []dynamic.WRRService) *dynamic.Service {
	return &dynamic.Service{
		Weighted: &dynamic.WeightedRoundRobin{
			Services: backendSvc,
		},
	}
}

func buildTCPServiceFromTrafficSplit(backendSvc []dynamic.TCPWRRService) *dynamic.TCPService {
	return &dynamic.TCPService{
		Weighted: &dynamic.TCPWeightedRoundRobin{
			Services: backendSvc,
		},
	}
}

func buildUDPServiceFromTrafficSplit(backendSvc []dynamic.UDPWRRService) *dynamic.UDPService {
	return &dynamic.UDPService{
		Weighted: &dynamic.UDPWeightedRoundRobin{
			Services: backendSvc,
		},
	}
}

func buildHTTPSplitTrafficBackendService(backend topology.TrafficSplitBackend, scheme string, port int32) *dynamic.Service {
	server := dynamic.Server{
		URL: fmt.Sprintf("%s://%s.%s.maesh:%d", scheme, backend.Service.Name, backend.Service.Namespace, port),
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers:        []dynamic.Server{server},
			PassHostHeader: getBoolRef(false),
		},
	}
}

func buildTCPSplitTrafficBackendService(backend topology.TrafficSplitBackend, port int32) *dynamic.TCPService {
	server := dynamic.TCPServer{
		Address: fmt.Sprintf("%s.%s.maesh:%d", backend.Service.Name, backend.Service.Namespace, port),
	}

	return &dynamic.TCPService{
		LoadBalancer: &dynamic.TCPServersLoadBalancer{
			Servers: []dynamic.TCPServer{server},
		},
	}
}

func buildUDPSplitTrafficBackendService(backend topology.TrafficSplitBackend, port int32) *dynamic.UDPService {
	server := dynamic.UDPServer{
		Address: fmt.Sprintf("%s.%s.maesh:%d", backend.Service.Name, backend.Service.Namespace, port),
	}

	return &dynamic.UDPService{
		LoadBalancer: &dynamic.UDPServersLoadBalancer{
			Servers: []dynamic.UDPServer{server},
		},
	}
}

func buildHTTPRouter(routerRule string, entrypoint string, middlewares []string, svcKey string, priority int) *dynamic.Router {
	return &dynamic.Router{
		EntryPoints: []string{entrypoint},
		Middlewares: middlewares,
		Service:     svcKey,
		Rule:        routerRule,
		Priority:    getRulePriority(routerRule, priority),
	}
}

func buildTCPRouter(routerRule string, entrypoint string, svcKey string) *dynamic.TCPRouter {
	return &dynamic.TCPRouter{
		EntryPoints: []string{entrypoint},
		Service:     svcKey,
		Rule:        routerRule,
	}
}

func buildUDPRouter(entrypoint string, svcKey string) *dynamic.UDPRouter {
	return &dynamic.UDPRouter{
		EntryPoints: []string{entrypoint},
		Service:     svcKey,
	}
}

func hasTrafficTargetRuleTCPRoute(tt *topology.ServiceTrafficTarget) bool {
	for _, rule := range tt.Rules {
		if rule.TCPRoute != nil {
			return true
		}
	}

	return false
}

func addToSliceCopy(items []string, item string) []string {
	cpy := make([]string, len(items)+1)
	copy(cpy, items)
	cpy[len(items)] = item

	return cpy
}

func addTCPService(config *dynamic.Configuration, key string, service *dynamic.TCPService) {
	if config.TCP == nil {
		config.TCP = &dynamic.TCPConfiguration{}
	}

	if config.TCP.Services == nil {
		config.TCP.Services = map[string]*dynamic.TCPService{}
	}

	config.TCP.Services[key] = service
}

func addTCPRouter(config *dynamic.Configuration, key string, router *dynamic.TCPRouter) {
	if config.TCP == nil {
		config.TCP = &dynamic.TCPConfiguration{}
	}

	if config.TCP.Routers == nil {
		config.TCP.Routers = map[string]*dynamic.TCPRouter{}
	}

	config.TCP.Routers[key] = router
}

func addUDPService(config *dynamic.Configuration, key string, service *dynamic.UDPService) {
	if config.UDP == nil {
		config.UDP = &dynamic.UDPConfiguration{}
	}

	if config.UDP.Services == nil {
		config.UDP.Services = map[string]*dynamic.UDPService{}
	}

	config.UDP.Services[key] = service
}

func addUDPRouter(config *dynamic.Configuration, key string, router *dynamic.UDPRouter) {
	if config.UDP == nil {
		config.UDP = &dynamic.UDPConfiguration{}
	}

	if config.UDP.Routers == nil {
		config.UDP.Routers = map[string]*dynamic.UDPRouter{}
	}

	config.UDP.Routers[key] = router
}

func getBoolRef(v bool) *bool {
	return &v
}

func getIntRef(v int) *int {
	return &v
}
