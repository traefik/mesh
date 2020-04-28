package provider

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// TCPPortFinder finds TCP port mappings.
type TCPPortFinder interface {
	Find(svc k8s.ServiceWithPort) (int32, bool)
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
	IgnoredResources   k8s.IgnoreWrapper
	MinHTTPPort        int32
	MaxHTTPPort        int32
	ACL                bool
	DefaultTrafficType string
	MaeshNamespace     string
}

// Provider holds the configuration for generating dynamic configuration from a kubernetes cluster state.
type Provider struct {
	config Config

	tcpStateTable          TCPPortFinder
	buildServiceMiddleware MiddlewareBuilder

	logger logrus.FieldLogger
}

// New creates a new Provider.
func New(tcpStateTable TCPPortFinder, cfg Config, logger logrus.FieldLogger) *Provider {
	return &Provider{
		config:                 cfg,
		tcpStateTable:          tcpStateTable,
		logger:                 logger,
		buildServiceMiddleware: buildMiddlewareFromAnnotations,
	}
}

// BuildConfig builds a dynamic configuration.
func (p *Provider) BuildConfig(t *topology.Topology) *dynamic.Configuration {
	cfg := buildDefaultDynamicConfig()

	for svcKey, svc := range t.Services {
		if err := p.buildConfigForService(t, cfg, svc); err != nil {
			p.logger.Errorf("Unable to build config for service %q: %w", svcKey, err)
		}
	}

	return cfg
}

// buildConfigForService builds the dynamic configuration for the given service.
func (p *Provider) buildConfigForService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service) error {
	trafficType, err := p.getTrafficTypeAnnotation(svc)
	if err != nil {
		return fmt.Errorf("unable to evaluate traffic-type annotation: %w", err)
	}

	scheme, err := getSchemeAnnotation(svc)
	if err != nil {
		return fmt.Errorf("unable to evaluate scheme annotation: %w", err)
	}

	var middlewares []string

	// Middlewares are currently supported only for HTTP services.
	if trafficType == k8s.ServiceTypeHTTP {
		middleware, err := p.buildServiceMiddleware(svc)
		if err != nil {
			return fmt.Errorf("unable to build middlewares: %w", err)
		}

		if middleware != nil {
			middlewareKey := getMiddlewareKey(svc)
			cfg.HTTP.Middlewares[middlewareKey] = middleware

			middlewares = append(middlewares, middlewareKey)
		}
	}

	// When ACL mode is on, all traffic must be forbidden unless explicitly authorized via a TrafficTarget.
	if p.config.ACL {
		if trafficType == k8s.ServiceTypeHTTP {
			p.buildBlockAllRouters(cfg, svc)
		}

		for _, ttKey := range svc.TrafficTargets {
			if err := p.buildServicesAndRoutersForTrafficTarget(t, cfg, ttKey, scheme, trafficType, middlewares); err != nil {
				p.logger.Errorf("Unable to build routers and services for traffic-target %q: %v", ttKey, err)
				continue
			}
		}
	} else {
		err := p.buildServicesAndRoutersForService(t, cfg, svc, scheme, trafficType, middlewares)
		if err != nil {
			return fmt.Errorf("unable to build routers and services: %w", err)
		}
	}

	for _, tsKey := range svc.TrafficSplits {
		if err := p.buildServiceAndRoutersForTrafficSplit(t, cfg, tsKey, scheme, trafficType, middlewares); err != nil {
			p.logger.Errorf("Unable to build routers and services for traffic-split %q: %v", tsKey, err)
			continue
		}
	}

	return nil
}

func (p *Provider) buildServicesAndRoutersForService(t *topology.Topology, cfg *dynamic.Configuration, svc *topology.Service, scheme, trafficType string, middlewares []string) error {
	svcKey := topology.Key{Name: svc.Name, Namespace: svc.Namespace}

	switch trafficType {
	case k8s.ServiceTypeHTTP:
		httpRule := buildHTTPRuleFromService(svc)

		for portID, svcPort := range svc.Ports {
			entrypoint, err := p.buildHTTPEntrypoint(portID)
			if err != nil {
				p.logger.Errorf("Unable to build HTTP entrypoint for Service %q and port %d: %v", svcKey, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(svc, svcPort.Port)
			cfg.HTTP.Services[key] = p.buildHTTPServiceFromService(t, svc, scheme, svcPort.TargetPort.IntVal)
			cfg.HTTP.Routers[key] = buildHTTPRouter(httpRule, entrypoint, middlewares, key, priorityService)
		}

	case k8s.ServiceTypeTCP:
		rule := buildTCPRouterRule()

		for _, svcPort := range svc.Ports {
			entrypoint, err := p.buildTCPEntrypoint(svc, svcPort.Port)
			if err != nil {
				p.logger.Errorf("Unable to build TCP entrypoint for Service %q and port %d: %v", svcKey, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(svc, svcPort.Port)
			cfg.TCP.Services[key] = p.buildTCPServiceFromService(t, svc, svcPort.TargetPort.IntVal)
			cfg.TCP.Routers[key] = buildTCPRouter(rule, entrypoint, key)
		}
	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
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
	case k8s.ServiceTypeHTTP:
		whitelistDirect := p.buildWhitelistMiddlewareFromTrafficTargetDirect(t, tt)
		whitelistDirectKey := getWhitelistMiddlewareKeyFromTrafficTargetDirect(tt)
		cfg.HTTP.Middlewares[whitelistDirectKey] = whitelistDirect

		rule := buildHTTPRuleFromTrafficTarget(tt, ttSvc)

		for portID, svcPort := range tt.Destination.Ports {
			entrypoint, err := p.buildHTTPEntrypoint(portID)
			if err != nil {
				p.logger.Errorf("Unable to build HTTP entrypoint for TrafficTarget %q and port %d: %v", ttKey, svcPort.Port, err)
				continue
			}

			svcKey := getServiceKeyFromTrafficTarget(tt, svcPort.Port)
			cfg.HTTP.Services[svcKey] = p.buildHTTPServiceFromTrafficTarget(t, tt, scheme, svcPort.TargetPort.IntVal)

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
	case k8s.ServiceTypeTCP:
		if !hasTrafficTargetSpecTCPRoute(tt) {
			return nil
		}

		rule := buildTCPRouterRule()

		for _, svcPort := range tt.Destination.Ports {
			entrypoint, err := p.buildTCPEntrypoint(ttSvc, svcPort.Port)
			if err != nil {
				p.logger.Errorf("Unable to build TCP entrypoint for TrafficTarget %q and port %d: %v", ttKey, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(ttSvc, svcPort.Port)
			cfg.TCP.Services[key] = p.buildTCPServiceFromTrafficTarget(t, tt, svcPort.TargetPort.IntVal)
			cfg.TCP.Routers[key] = buildTCPRouter(rule, entrypoint, key)
		}
	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
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
	case k8s.ServiceTypeHTTP:
		rule := buildHTTPRuleFromService(tsSvc)

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
				p.logger.Errorf("Unable to build HTTP backend services for TrafficSplit %q and port %d: %v", tsKey, svcPort.Port, err)
				continue
			}

			entrypoint, err := p.buildHTTPEntrypoint(portID)
			if err != nil {
				p.logger.Errorf("Unable to build HTTP entrypoint for TrafficSplit %q and port %d: %v", tsKey, svcPort.Port, err)
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

				rule = buildHTTPRuleFromTrafficSplitIndirect(tsSvc)
				rtrMiddlewaresindirect := addToSliceCopy(middlewares, whitelistIndirectKey)

				indirectRtrKey := getRouterKeyFromTrafficSplitIndirect(ts, svcPort.Port)
				cfg.HTTP.Routers[indirectRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewaresindirect, svcKey, priorityTrafficTargetIndirect)
			}
		}
	case k8s.ServiceTypeTCP:
		tcpRule := buildTCPRouterRule()

		for _, svcPort := range tsSvc.Ports {
			backendSvcs := make([]dynamic.TCPWRRService, len(ts.Backends))

			for i, backend := range ts.Backends {
				backendSvcKey := getServiceKeyFromTrafficSplitBackend(ts, svcPort.Port, backend)
				cfg.TCP.Services[backendSvcKey] = buildTCPSplitTrafficBackendService(backend, svcPort.TargetPort.IntVal)
				backendSvcs[i] = dynamic.TCPWRRService{
					Name:   backendSvcKey,
					Weight: getIntRef(backend.Weight),
				}
			}

			entrypoint, err := p.buildTCPEntrypoint(tsSvc, svcPort.Port)
			if err != nil {
				p.logger.Errorf("Unable to build TCP entrypoint for TrafficTarget %q and port %d: %v", tsKey, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(tsSvc, svcPort.Port)
			cfg.TCP.Services[key] = buildTCPServiceFromTrafficSplit(backendSvcs)
			cfg.TCP.Routers[key] = buildTCPRouter(tcpRule, entrypoint, key)
		}

	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
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
			p.logger.Errorf("unable to build HTTP entrypoint for Service %q and port %d: %w", svcKey, svcPort.Port, err)

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
	if port >= p.config.MaxHTTPPort {
		return "", errors.New("too many HTTP entrypoints")
	}

	return fmt.Sprintf("http-%d", port), nil
}

func (p Provider) buildTCPEntrypoint(svc *topology.Service, port int32) (string, error) {
	meshPort, ok := p.tcpStateTable.Find(k8s.ServiceWithPort{
		Namespace: svc.Namespace,
		Name:      svc.Name,
		Port:      port,
	})

	if !ok {
		return "", errors.New("port not found")
	}

	return fmt.Sprintf("tcp-%d", meshPort), nil
}

func (p *Provider) getTrafficTypeAnnotation(svc *topology.Service) (string, error) {
	trafficType, ok := svc.Annotations[k8s.AnnotationServiceType]

	if !ok {
		return p.config.DefaultTrafficType, nil
	}

	if trafficType != k8s.ServiceTypeHTTP && trafficType != k8s.ServiceTypeTCP {
		return "", fmt.Errorf("traffic-type annotation references an unsupported traffic type %q", trafficType)
	}

	return trafficType, nil
}

func (p *Provider) buildHTTPServiceFromService(t *topology.Topology, svc *topology.Service, scheme string, port int32) *dynamic.Service {
	var servers []dynamic.Server

	for _, podKey := range svc.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q", podKey)
			continue
		}

		url := net.JoinHostPort(pod.IP, strconv.Itoa(int(port)))

		servers = append(servers, dynamic.Server{
			URL: fmt.Sprintf("%s://%s", scheme, url),
		})
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers:        servers,
			PassHostHeader: getBoolRef(true),
		},
	}
}

func (p *Provider) buildHTTPServiceFromTrafficTarget(t *topology.Topology, tt *topology.ServiceTrafficTarget, scheme string, port int32) *dynamic.Service {
	servers := make([]dynamic.Server, len(tt.Destination.Pods))

	for i, podKey := range tt.Destination.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q", podKey)
			continue
		}

		url := net.JoinHostPort(pod.IP, strconv.Itoa(int(port)))

		servers[i].URL = fmt.Sprintf("%s://%s", scheme, url)
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers:        servers,
			PassHostHeader: getBoolRef(true),
		},
	}
}

func (p *Provider) buildTCPServiceFromService(t *topology.Topology, svc *topology.Service, port int32) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	for _, podKey := range svc.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q", podKey)
			continue
		}

		address := net.JoinHostPort(pod.IP, strconv.Itoa(int(port)))

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

func (p *Provider) buildTCPServiceFromTrafficTarget(t *topology.Topology, tt *topology.ServiceTrafficTarget, port int32) *dynamic.TCPService {
	servers := make([]dynamic.TCPServer, len(tt.Destination.Pods))

	for i, podKey := range tt.Destination.Pods {
		pod, ok := t.Pods[podKey]
		if !ok {
			p.logger.Errorf("Unable to find Pod %q", podKey)
			continue
		}

		servers[i].Address = net.JoinHostPort(pod.IP, strconv.Itoa(int(port)))
	}

	return &dynamic.TCPService{
		LoadBalancer: &dynamic.TCPServersLoadBalancer{
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
				p.logger.Errorf("Unable to find Pod %q", podKey)
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
			p.logger.Errorf("Unable to find Pod %q", podKey)
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

func getSchemeAnnotation(svc *topology.Service) (string, error) {
	scheme, ok := svc.Annotations[k8s.AnnotationScheme]

	if !ok {
		return k8s.SchemeHTTP, nil
	}

	if scheme != k8s.SchemeHTTP && scheme != k8s.SchemeH2c && scheme != k8s.SchemeHTTPS {
		return "", fmt.Errorf("scheme annotation references an unknown scheme %q", scheme)
	}

	return scheme, nil
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

func hasTrafficTargetSpecTCPRoute(tt *topology.ServiceTrafficTarget) bool {
	for _, spec := range tt.Specs {
		if spec.TCPRoute != nil {
			return true
		}
	}

	return false
}

func buildDefaultDynamicConfig() *dynamic.Configuration {
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
					LoadBalancer: &dynamic.ServersLoadBalancer{},
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
		TCP: &dynamic.TCPConfiguration{
			Routers:  map[string]*dynamic.TCPRouter{},
			Services: map[string]*dynamic.TCPService{},
		},
	}
}

func addToSliceCopy(items []string, item string) []string {
	cpy := make([]string, len(items)+1)
	copy(cpy, items)
	cpy[len(items)] = item

	return cpy
}

func getBoolRef(v bool) *bool {
	return &v
}

func getIntRef(v int) *int {
	return &v
}
