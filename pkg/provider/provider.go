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
)

// TopologyBuilder is capable of building Topologies.
type TopologyBuilder interface {
	Build(ignored k8s.IgnoreWrapper) (*topology.Topology, error)
}

// TCPPortFinder is capable for finding a TCP port mapping.
type TCPPortFinder interface {
	Find(svc k8s.ServiceWithPort) (int32, bool)
}

// When multiple Traefik Routers listen to the same entrypoint and have the same Rule, the chosen router will be the one
// with the highest priority. There are few cases where this priority is crucial when building the dynamic configuration:
// - When a TrafficSplit is set on a k8s service, this will create 2 Traefik Routers. One for accessing the k8s service
//   endpoints and one for accessing the services endpoints mentioned in the TrafficSplit. They both have the same Rule
//   but we should always prioritize the TrafficSplit. Therefore, TrafficSplit Routers should always have a higher priority.
// - When a TrafficTarget Destination targets pods of a k8s service and a TrafficSplit is set on this service. This
//   will create 2 Traefik Routers. One for the TrafficSplit and one for the TrafficTarget. We should always prioritize
//   TrafficSplits Routers and so, TrafficSplit Routers should always have a higher priority than TrafficTarget Routers.
const (
	priorityService               = 1
	priorityTrafficTargetDirect   = 2
	priorityTrafficTargetIndirect = 3
	priorityTrafficSplit          = 4
)

// Provider holds the configuration for generating dynamic configuration from a kubernetes cluster state.
type Provider struct {
	acl                bool
	minHTTPPort        int32
	maxHTTPPort        int32
	defaultTrafficType string
	maeshNamespace     string

	topologyBuilder   TopologyBuilder
	tcpStateTable     TCPPortFinder
	middlewareBuilder MiddlewareBuilder
	ignored           k8s.IgnoreWrapper

	logger logrus.FieldLogger
}

// New creates a new Provider.
func New(topologyBuilder TopologyBuilder, tcpStateTable TCPPortFinder, ignored k8s.IgnoreWrapper, minHTTPPort, maxHTTPPort int32, acl bool, defaultTrafficType, maeshNamespace string, logger logrus.FieldLogger) *Provider {
	return &Provider{
		acl:                acl,
		minHTTPPort:        minHTTPPort,
		maxHTTPPort:        maxHTTPPort,
		defaultTrafficType: defaultTrafficType,
		maeshNamespace:     maeshNamespace,
		middlewareBuilder:  &AnnotationBasedMiddlewareBuilder{},
		topologyBuilder:    topologyBuilder,
		tcpStateTable:      tcpStateTable,
		ignored:            ignored,
		logger:             logger,
	}
}

// BuildConfig builds a dynamic configuration.
func (p *Provider) BuildConfig() (*dynamic.Configuration, error) {
	cfg := buildDefaultDynamicConfig()

	t, err := p.topologyBuilder.Build(p.ignored)
	if err != nil {
		return nil, fmt.Errorf("unable to build topology: %w", err)
	}

	for _, svc := range t.Services {
		if err := p.buildConfigForService(cfg, svc); err != nil {
			p.logger.Errorf("Unable to build config for service %s/%s: %w", svc.Namespace, svc.Name, err)
		}
	}

	return cfg, nil
}

// buildConfigForService builds the dynamic configuration for the given service.
func (p *Provider) buildConfigForService(cfg *dynamic.Configuration, svc *topology.Service) error {
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
		middleware, err := p.middlewareBuilder.Build(svc)
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
	if p.acl {
		if trafficType == k8s.ServiceTypeHTTP {
			p.buildBlockAllRouters(cfg, svc)
		}

		for _, tt := range svc.TrafficTargets {
			if err := p.buildServicesAndRoutersForTrafficTarget(cfg, tt, scheme, trafficType, middlewares); err != nil {
				p.logger.Errorf("Unable to build routers and services for service %s/%s for traffic-target %s: %v", svc.Namespace, svc.Name, tt.Name, err)
				continue
			}
		}
	} else {
		err := p.buildServicesAndRoutersForService(cfg, svc, scheme, trafficType, middlewares)
		if err != nil {
			return fmt.Errorf("unable to build routers and services: %w", err)
		}
	}

	for _, ts := range svc.TrafficSplits {
		if err := p.buildServiceAndRoutersForTrafficSplit(cfg, ts, scheme, trafficType, middlewares); err != nil {
			p.logger.Errorf("Unable to build routers and services for service %s/%s and traffic-split %s: %v", svc.Namespace, svc.Name, ts.Name, err)
			continue
		}
	}

	return nil
}

func (p *Provider) buildServicesAndRoutersForService(cfg *dynamic.Configuration, svc *topology.Service, scheme, trafficType string, middlewares []string) error {
	switch trafficType {
	case k8s.ServiceTypeHTTP:
		httpRule := fmt.Sprintf("Host(`%s.%s.maesh`) || Host(`%s`)", svc.Name, svc.Namespace, svc.ClusterIP)

		for portID, svcPort := range svc.Ports {
			entrypoint, err := p.buildHTTPEntrypoint(portID)
			if err != nil {
				p.logger.Errorf("Unable to build HTTP entrypoint for Service %s/%s and portID %d: %v", svc.Namespace, svc.Name, portID, err)
				continue
			}

			key := getServiceRouterKeyFromService(svc, svcPort.Port)
			cfg.HTTP.Services[key] = buildHTTPServiceFromService(svc, scheme, svcPort.TargetPort.IntVal)
			cfg.HTTP.Routers[key] = buildHTTPRouter(httpRule, entrypoint, middlewares, key, priorityService)
		}

	case k8s.ServiceTypeTCP:
		rule := buildTCPRouterRule()

		for _, svcPort := range svc.Ports {
			entrypoint, err := p.buildTCPEntrypoint(svc, svcPort.Port)
			if err != nil {
				p.logger.Errorf("Unable to build TCP entrypoint for Service %s/%s and port %d: %v", svc.Namespace, svc.Name, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(svc, svcPort.Port)
			cfg.TCP.Services[key] = buildTCPServiceFromService(svc, svcPort.TargetPort.IntVal)
			cfg.TCP.Routers[key] = buildTCPRouter(rule, entrypoint, key)
		}
	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
}

func (p *Provider) buildServicesAndRoutersForTrafficTarget(cfg *dynamic.Configuration, tt *topology.ServiceTrafficTarget, scheme, trafficType string, middlewares []string) error {
	switch trafficType {
	case k8s.ServiceTypeHTTP:
		whitelistDirect := buildWhitelistMiddlewareFromTrafficTargetDirect(tt)
		whitelistDirectKey := getWhitelistMiddlewareKeyFromTrafficTargetDirect(tt)
		cfg.HTTP.Middlewares[whitelistDirectKey] = whitelistDirect

		rule := buildHTTPRuleFromTrafficTarget(tt)

		for portID, svcPort := range tt.Destination.Ports {
			entrypoint, err := p.buildHTTPEntrypoint(portID)
			if err != nil {
				p.logger.Errorf("Unable to build HTTP entrypoint for Service %s/%s and portID %d: %v", tt.Service.Namespace, tt.Service.Name, portID, err)
				continue
			}

			svcKey := getServiceKeyFromTrafficTarget(tt, svcPort.Port)
			cfg.HTTP.Services[svcKey] = buildHTTPServiceFromTrafficTarget(tt, scheme, svcPort.TargetPort.IntVal)

			rtrMiddlewares := addToSliceCopy(middlewares, whitelistDirectKey)

			directRtrKey := getRouterKeyFromTrafficTargetDirect(tt, svcPort.Port)
			cfg.HTTP.Routers[directRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewares, svcKey, priorityTrafficTargetDirect)

			// If the ServiceTrafficTarget is a backend of at least one TrafficSplit we need an additional router with
			// a whitelist middleware which whitelists based on the X-Forwarded-For header instead of on the RemoteAddr value.
			if len(tt.Service.BackendOf) > 0 {
				whitelistIndirect := buildWhitelistMiddlewareFromTrafficTargetIndirect(tt)
				whitelistIndirectKey := getWhitelistMiddlewareKeyFromTrafficTargetIndirect(tt)
				cfg.HTTP.Middlewares[whitelistIndirectKey] = whitelistIndirect

				rule = buildHTTPRuleFromTrafficTargetIndirect(tt)
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
			entrypoint, err := p.buildTCPEntrypoint(tt.Service, svcPort.Port)
			if err != nil {
				p.logger.Errorf("Unable to build TCP entrypoint for Service %s/%s and port %d: %v", tt.Service.Namespace, tt.Service.Name, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(tt.Service, svcPort.Port)
			cfg.TCP.Services[key] = buildTCPServiceFromTrafficTarget(tt, svcPort.TargetPort.IntVal)
			cfg.TCP.Routers[key] = buildTCPRouter(rule, entrypoint, key)
		}
	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
}

func (p *Provider) buildServiceAndRoutersForTrafficSplit(cfg *dynamic.Configuration, ts *topology.TrafficSplit, scheme, trafficType string, middlewares []string) error {
	switch trafficType {
	case k8s.ServiceTypeHTTP:
		rule := buildHTTPRuleFromService(ts.Service)

		rtrMiddlewares := middlewares

		if p.acl {
			whitelistDirect := buildWhitelistMiddlewareFromTrafficSplitDirect(ts)
			whitelistDirectKey := getWhitelistMiddlewareKeyFromTrafficSplitDirect(ts)
			cfg.HTTP.Middlewares[whitelistDirectKey] = whitelistDirect

			rtrMiddlewares = addToSliceCopy(middlewares, whitelistDirectKey)
		}

		for portID, svcPort := range ts.Service.Ports {
			backendSvcs := make([]dynamic.WRRService, len(ts.Backends))

			for i, backend := range ts.Backends {
				if len(backend.Service.TrafficSplits) > 0 {
					p.logger.Warnf("Nested TrafficSplits detected in TrafficSplit %s/%s: Maesh doesn't support nested TrafficSplits", ts.Namespace, ts.Name)
				}

				backendSvcKey := getServiceKeyFromTrafficSplitBackend(ts, svcPort.Port, backend)
				// This is unclear in SMI's specification if port mapping should be enforced between the Service and the
				// TrafficSplit backends. https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-split.md#ports
				cfg.HTTP.Services[backendSvcKey] = buildHTTPSplitTrafficBackendService(backend, scheme, svcPort.Port)
				backendSvcs[i] = dynamic.WRRService{
					Name:   backendSvcKey,
					Weight: getIntRef(backend.Weight),
				}
			}

			entrypoint, err := p.buildHTTPEntrypoint(portID)
			if err != nil {
				p.logger.Errorf("Unable to build HTTP entrypoint for Service %s/%s and portID %d: %v", ts.Service.Namespace, ts.Service.Name, portID, err)
				continue
			}

			svcKey := getServiceKeyFromTrafficSplit(ts, svcPort.Port)
			cfg.HTTP.Services[svcKey] = buildHTTPServiceFromTrafficSplit(backendSvcs)

			directRtrKey := getRouterKeyFromTrafficSplitDirect(ts, svcPort.Port)
			cfg.HTTP.Routers[directRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewares, svcKey, priorityTrafficSplit)

			// If the ServiceTrafficSplit is a backend of at least one TrafficSplit we need an additional router with
			// a whitelist middleware which whitelists based on the X-Forwarded-For header instead of on the RemoteAddr value.
			if len(ts.Service.BackendOf) > 0 && p.acl {
				whitelistIndirect := buildWhitelistMiddlewareFromTrafficSplitIndirect(ts)
				whitelistIndirectKey := getWhitelistMiddlewareKeyFromTrafficSplitIndirect(ts)
				cfg.HTTP.Middlewares[whitelistIndirectKey] = whitelistIndirect

				rule = buildHTTPRuleFromTrafficSplitIndirect(ts)
				rtrMiddlewaresindirect := addToSliceCopy(middlewares, whitelistIndirectKey)

				indirectRtrKey := getRouterKeyFromTrafficSplitIndirect(ts, svcPort.Port)
				cfg.HTTP.Routers[indirectRtrKey] = buildHTTPRouter(rule, entrypoint, rtrMiddlewaresindirect, svcKey, priorityTrafficTargetIndirect)
			}
		}
	case k8s.ServiceTypeTCP:
		tcpRule := buildTCPRouterRule()

		for _, svcPort := range ts.Service.Ports {
			backendSvcs := make([]dynamic.TCPWRRService, len(ts.Backends))

			for i, backend := range ts.Backends {
				backendSvcKey := getServiceKeyFromTrafficSplitBackend(ts, svcPort.Port, backend)
				cfg.TCP.Services[backendSvcKey] = buildTCPSplitTrafficBackendService(backend, svcPort.TargetPort.IntVal)
				backendSvcs[i] = dynamic.TCPWRRService{
					Name:   backendSvcKey,
					Weight: getIntRef(backend.Weight),
				}
			}

			entrypoint, err := p.buildTCPEntrypoint(ts.Service, svcPort.Port)
			if err != nil {
				p.logger.Errorf("Unable to build TCP entrypoint for Service %s/%s and port %d: %v", ts.Service.Namespace, ts.Service.Name, svcPort.Port, err)
				continue
			}

			key := getServiceRouterKeyFromService(ts.Service, svcPort.Port)
			cfg.TCP.Services[key] = buildTCPServiceFromTrafficSplit(backendSvcs)
			cfg.TCP.Routers[key] = buildTCPRouter(tcpRule, entrypoint, key)
		}

	default:
		return fmt.Errorf("unknown traffic-type %q", trafficType)
	}

	return nil
}

func (p *Provider) buildBlockAllRouters(cfg *dynamic.Configuration, svc *topology.Service) {
	rule := buildHTTPRuleFromService(svc)

	for portID, svcPort := range svc.Ports {
		entrypoint, err := p.buildHTTPEntrypoint(portID)
		if err != nil {
			p.logger.Errorf("unable to build HTTP entrypoint for Service %s/%s and portID %d: %w", svc.Namespace, svc.Name, portID, err)
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
	port := p.minHTTPPort + int32(portID)
	if port >= p.maxHTTPPort {
		return "", errors.New("too many HTTP entrypoint")
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
		return p.defaultTrafficType, nil
	}

	if trafficType != k8s.ServiceTypeHTTP && trafficType != k8s.ServiceTypeTCP {
		return "", fmt.Errorf("traffic-type annotation references an unknown traffic type %q", trafficType)
	}

	return trafficType, nil
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

func buildHTTPServiceFromService(svc *topology.Service, scheme string, port int32) *dynamic.Service {
	var servers []dynamic.Server

	for _, pod := range svc.Pods {
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

func buildTCPServiceFromService(svc *topology.Service, port int32) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	for _, pod := range svc.Pods {
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

func buildHTTPServiceFromTrafficTarget(tt *topology.ServiceTrafficTarget, scheme string, port int32) *dynamic.Service {
	servers := make([]dynamic.Server, len(tt.Destination.Pods))

	for i, pod := range tt.Destination.Pods {
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

func buildTCPServiceFromTrafficTarget(tt *topology.ServiceTrafficTarget, port int32) *dynamic.TCPService {
	servers := make([]dynamic.TCPServer, len(tt.Destination.Pods))

	for i, pod := range tt.Destination.Pods {
		servers[i].Address = net.JoinHostPort(pod.IP, strconv.Itoa(int(port)))
	}

	return &dynamic.TCPService{
		LoadBalancer: &dynamic.TCPServersLoadBalancer{
			Servers: servers,
		},
	}
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
