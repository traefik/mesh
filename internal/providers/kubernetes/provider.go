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
)

// Provider holds a client to access the provider.
type Provider struct {
	client        k8s.CoreV1Client
	defaultMode   string
	meshNamespace string
	tcpStateTable *k8s.State
	ignored       k8s.IgnoreWrapper
}

// Init the provider.
func (p *Provider) Init() {

}

// New creates a new provider.
func New(client k8s.CoreV1Client, defaultMode string, meshNamespace string, tcpStateTable *k8s.State, ignored k8s.IgnoreWrapper) *Provider {
	p := &Provider{
		client:        client,
		defaultMode:   defaultMode,
		meshNamespace: meshNamespace,
		tcpStateTable: tcpStateTable,
		ignored:       ignored,
	}

	p.Init()

	return p
}

func (p *Provider) buildRouter(name, namespace, ip string, port int, serviceName string, addMiddlewares bool) *dynamic.Router {
	if addMiddlewares {
		return &dynamic.Router{
			Rule:        fmt.Sprintf("Host(`%s.%s.%s`) || Host(`%s`)", name, namespace, p.meshNamespace, ip),
			EntryPoints: []string{fmt.Sprintf("http-%d", port)},
			Middlewares: []string{serviceName},
			Service:     serviceName,
		}
	}

	return &dynamic.Router{
		Rule:        fmt.Sprintf("Host(`%s.%s.%s`) || Host(`%s`)", name, namespace, p.meshNamespace, ip),
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

func (p *Provider) buildService(endpoints *corev1.Endpoints, scheme string) *dynamic.Service {
	var servers []dynamic.Server

	if endpoints.Subsets != nil {
		for _, subset := range endpoints.Subsets {
			for _, endpointPort := range subset.Ports {
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
		PassHostHeader: true,
		Servers:        servers,
	}

	return &dynamic.Service{
		LoadBalancer: lb,
	}
}

func (p *Provider) buildTCPService(endpoints *corev1.Endpoints) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	if endpoints.Subsets != nil {
		for _, subset := range endpoints.Subsets {
			for _, endpointPort := range subset.Ports {
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
	services, err := p.client.GetServices(metav1.NamespaceAll)
	if err != nil {
		return nil, fmt.Errorf("unable to get services: %v", err)
	}

	endpoints, err := p.client.GetEndpointses(metav1.NamespaceAll)
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoints: %v", err)
	}

	config := base.CreateBaseConfigWithReadiness()

	for _, service := range services {
		if p.ignored.Ignored(service.Name, service.Namespace) {
			continue
		}

		serviceMode := p.getServiceMode(service.Annotations)
		scheme := p.getScheme(service.Annotations)

		for id, sp := range service.Spec.Ports {
			key := buildKey(service.Name, service.Namespace, sp.Port)

			if serviceMode == k8s.ServiceTypeHTTP {
				config.HTTP.Services[key] = p.buildService(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), scheme)
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
			config.TCP.Services[key] = p.buildTCPService(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints))
		}
	}

	return config, nil
}

func (p *Provider) getServiceMode(annotations map[string]string) string {
	mode := annotations[k8s.AnnotationServiceType]

	if mode == "" {
		return p.defaultMode
	}

	return mode
}

func (p *Provider) getScheme(annotations map[string]string) string {
	scheme := annotations[k8s.AnnotationScheme]

	if scheme == "" {
		return "http"
	}

	return scheme
}

func (p *Provider) buildHTTPMiddlewares(annotations map[string]string) *dynamic.Middleware {
	circuitBreaker := buildCircuitBreakerMiddleware(annotations)
	retry := buildRetryMiddleware(annotations)

	if circuitBreaker == nil && retry == nil {
		return nil
	}

	return &dynamic.Middleware{
		CircuitBreaker: circuitBreaker,
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

func (p *Provider) getMeshPort(serviceName, serviceNamespace string, servicePort int32) int {
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
