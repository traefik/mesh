package kubernetes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/providers/base"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	defaultMode     string
	tcpStateTable   *k8s.State
	ignored         k8s.IgnoreWrapper
	serviceLister   listers.ServiceLister
	endpointsLister listers.EndpointsLister
}

// Init the provider.
func (p *Provider) Init() {

}

// New creates a new provider.
func New(defaultMode string, tcpStateTable *k8s.State, ignored k8s.IgnoreWrapper, serviceLister listers.ServiceLister, endpointsLister listers.EndpointsLister) *Provider {
	p := &Provider{
		defaultMode:     defaultMode,
		tcpStateTable:   tcpStateTable,
		ignored:         ignored,
		serviceLister:   serviceLister,
		endpointsLister: endpointsLister,
	}

	p.Init()

	return p
}

func (p *Provider) buildRouter(name, namespace, ip string, port int, serviceName string, addMiddlewares bool) *dynamic.Router {
	if addMiddlewares {
		return &dynamic.Router{
			Rule:        fmt.Sprintf("Host(`%s.%s.maesh`) || Host(`%s`)", name, namespace, ip),
			EntryPoints: []string{fmt.Sprintf("http-%d", port)},
			Middlewares: []string{serviceName},
			Service:     serviceName,
		}
	}

	return &dynamic.Router{
		Rule:        fmt.Sprintf("Host(`%s.%s.maesh`) || Host(`%s`)", name, namespace, ip),
		EntryPoints: []string{fmt.Sprintf("http-%d", port)},
		Service:     serviceName,
	}
}

func (p *Provider) buildTCPRouter(port int, serviceName string) *dynamic.TCPRouter {
	return &dynamic.TCPRouter{
		Rule:        "HostSNI(`*`)",
		EntryPoints: []string{fmt.Sprintf("tcp-%d", port)},
		Service:     serviceName,
	}
}

func (p *Provider) buildService(endpoints *corev1.Endpoints, scheme string, servicePort int32) *dynamic.Service {
	var servers []dynamic.Server

	if endpoints != nil && endpoints.Subsets != nil {
		for _, subset := range endpoints.Subsets {
			for _, endpointPort := range subset.Ports {
				if endpointPort.Port != servicePort {
					continue
				}

				for _, address := range subset.Addresses {
					server := dynamic.Server{
						URL: fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10))),
					}
					servers = append(servers, server)
				}
			}
		}
	}

	lb := &dynamic.ServersLoadBalancer{
		PassHostHeader: base.Bool(true),
		Servers:        servers,
	}

	return &dynamic.Service{
		LoadBalancer: lb,
	}
}

func (p *Provider) buildTCPService(endpoints *corev1.Endpoints, servicePort int32) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	if endpoints != nil && endpoints.Subsets != nil {
		for _, subset := range endpoints.Subsets {
			for _, endpointPort := range subset.Ports {
				if endpointPort.Port != servicePort {
					continue
				}

				for _, address := range subset.Addresses {
					server := dynamic.TCPServer{
						Address: net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10)),
					}
					servers = append(servers, server)
				}
			}
		}
	}

	lb := &dynamic.TCPServersLoadBalancer{
		Servers: servers,
	}

	return &dynamic.TCPService{
		LoadBalancer: lb,
	}
}

// BuildConfig builds the configuration for routing
// from a native kubernetes environment.
func (p *Provider) BuildConfig() (*dynamic.Configuration, error) {
	services, err := p.serviceLister.Services(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to get services: %v", err)
	}

	endpoints, err := p.endpointsLister.Endpoints(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoints: %v", err)
	}

	config := base.CreateBaseConfigWithReadiness()

	for _, service := range services {
		if p.ignored.IsIgnored(service.ObjectMeta) {
			continue
		}

		serviceMode := base.GetServiceMode(service.Annotations, p.defaultMode)
		scheme := base.GetScheme(service.Annotations)

		for id, sp := range service.Spec.Ports {
			key := buildKey(service.Name, service.Namespace, sp.Port)

			if serviceMode == k8s.ServiceTypeHTTP {
				config.HTTP.Services[key] = p.buildService(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), scheme, sp.Port)
				middlewares := p.buildHTTPMiddlewares(service.Annotations)

				if middlewares != nil {
					config.HTTP.Routers[key] = p.buildRouter(service.Name, service.Namespace, service.Spec.ClusterIP, 5000+id, key, true)
					config.HTTP.Middlewares[key] = middlewares

					continue
				}

				config.HTTP.Routers[key] = p.buildRouter(service.Name, service.Namespace, service.Spec.ClusterIP, 5000+id, key, false)

				continue
			}

			meshPort := p.getMeshPort(service.Name, service.Namespace, sp.Port)
			config.TCP.Routers[key] = p.buildTCPRouter(meshPort, key)
			config.TCP.Services[key] = p.buildTCPService(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), sp.Port)
		}
	}

	return config, nil
}

func (p *Provider) buildHTTPMiddlewares(annotations map[string]string) *dynamic.Middleware {
	circuitBreaker := buildCircuitBreakerMiddleware(annotations)
	retry := buildRetryMiddleware(annotations)
	rateLimit := buildRateLimitMiddleware(annotations)

	if circuitBreaker == nil && retry == nil && rateLimit == nil {
		return nil
	}

	return &dynamic.Middleware{
		CircuitBreaker: circuitBreaker,
		RateLimit:      rateLimit,
		Retry:          retry,
	}
}

func buildCircuitBreakerMiddleware(annotations map[string]string) *dynamic.CircuitBreaker {
	if annotations[k8s.AnnotationCircuitBreakerExpression] != "" {
		expression := annotations[k8s.AnnotationCircuitBreakerExpression]
		if expression != "" {
			return &dynamic.CircuitBreaker{
				Expression: expression,
			}
		}
	}

	return nil
}

func buildRetryMiddleware(annotations map[string]string) *dynamic.Retry {
	if annotations[k8s.AnnotationRetryAttempts] != "" {
		retryAttempts, err := strconv.Atoi(annotations[k8s.AnnotationRetryAttempts])
		if err != nil {
			log.Errorf("Could not parse retry annotation: %v", err)
		}

		if retryAttempts > 0 {
			return &dynamic.Retry{
				Attempts: retryAttempts,
			}
		}
	}

	return nil
}

func buildRateLimitMiddleware(annotations map[string]string) *dynamic.RateLimit {
	if annotations[k8s.AnnotationRateLimitAverage] != "" || annotations[k8s.AnnotationRateLimitBurst] != "" {
		rlAverage, err := strconv.Atoi(annotations[k8s.AnnotationRateLimitAverage])
		if err != nil {
			log.Errorf("Could not parse rateLimit average annotation: %v", err)
		}

		rlBurst, err := strconv.Atoi(annotations[k8s.AnnotationRateLimitBurst])
		if err != nil {
			log.Errorf("Could not parse rateLimit burst annotation: %v", err)
		}

		if rlAverage > 0 && rlBurst > 1 {
			return &dynamic.RateLimit{
				Average: int64(rlAverage),
				Burst:   int64(rlBurst),
			}
		}
	}

	return nil
}

func (p *Provider) getMeshPort(serviceName, serviceNamespace string, servicePort int32) int {
	if p.tcpStateTable == nil {
		return 0
	}

	for port, v := range p.tcpStateTable.Table {
		if v.Name == serviceName && v.Namespace == serviceNamespace && v.Port == servicePort {
			return port
		}
	}

	return 0
}

func buildKey(name, namespace string, port int32) string {
	// Use the hash of the servicename.namespace.port as the key
	// So that we can update services based on their name
	// and not have to worry about duplicates on merges.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s.%s.%d", name, namespace, port)))
	dst := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(dst, sum[:])
	fullHash := string(dst)

	return fmt.Sprintf("%.10s-%.10s-%d-%.16s", name, namespace, port, fullHash)
}
