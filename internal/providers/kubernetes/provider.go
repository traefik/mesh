package kubernetes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/message"
	"github.com/containous/traefik/pkg/config/dynamic"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	client        k8s.CoreV1Client
	defaultMode   string
	meshNamespace string
	tcpStateTable *k8s.State
}

// Init the provider.
func (p *Provider) Init() {

}

// New creates a new provider.
func New(client k8s.CoreV1Client, defaultMode string, meshNamespace string, tcpStateTable *k8s.State) *Provider {
	p := &Provider{
		client:        client,
		defaultMode:   defaultMode,
		meshNamespace: meshNamespace,
		tcpStateTable: tcpStateTable,
	}

	p.Init()

	return p
}

// BuildConfiguration builds the configuration for routing
// from a native kubernetes environment.
func (p *Provider) BuildConfiguration(event message.Message, traefikConfig *dynamic.Configuration) {
	switch obj := event.Object.(type) {
	case *corev1.Service:
		switch event.Action {
		case message.TypeCreated:
			p.buildServiceIntoConfig(obj, nil, traefikConfig)
		case message.TypeUpdated:
			//FIXME: We will need to delete the old references in the config, and create the new service.
		case message.TypeDeleted:
			p.deleteServiceFromConfig(obj, traefikConfig)
		}
	case *corev1.Endpoints:
		switch event.Action {
		case message.TypeCreated:
			// We don't process created endpoint events, processing is done under service creation.
		case message.TypeUpdated:
			p.buildServiceIntoConfig(nil, obj, traefikConfig)
		case message.TypeDeleted:
			// We don't precess deleted endpoint events, processig is done under service deletion.
		}
	}

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

func (p *Provider) buildService(endpoints *corev1.Endpoints) *dynamic.Service {
	var servers []dynamic.Server
	for _, subset := range endpoints.Subsets {
		for _, endpointPort := range subset.Ports {
			for _, address := range subset.Addresses {
				server := dynamic.Server{
					URL: "http://" + net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10)),
				}
				servers = append(servers, server)
			}
		}
	}

	lb := &dynamic.LoadBalancerService{
		PassHostHeader: true,
		Servers:        servers,
	}

	return &dynamic.Service{
		LoadBalancer: lb,
	}
}

func (p *Provider) buildTCPService(endpoints *corev1.Endpoints) *dynamic.TCPService {
	var servers []dynamic.TCPServer
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

	lb := &dynamic.TCPLoadBalancerService{
		Servers: servers,
	}

	return &dynamic.TCPService{
		LoadBalancer: lb,
	}
}

func (p *Provider) buildServiceIntoConfig(service *corev1.Service, endpoints *corev1.Endpoints, config *dynamic.Configuration) {
	var exists bool
	var err error
	if service == nil {
		service, exists, err = p.client.GetService(endpoints.Namespace, endpoints.Name)
		if err != nil {
			log.Errorf("Could not get service %s/%s: %v", endpoints.Namespace, endpoints.Name, err)
			return
		}
		if !exists {
			log.Errorf("endpoints for service %s/%s do not exist", endpoints.Namespace, endpoints.Name)
			return
		}

	}

	if endpoints == nil {
		endpoints, exists, err = p.client.GetEndpoints(service.Namespace, service.Name)
		if err != nil {
			log.Errorf("Could not get endpoints for service %s/%s: %v", service.Namespace, service.Name, err)
			return
		}
		if !exists {
			log.Errorf("endpoints for service %s/%s do not exist", service.Namespace, service.Name)
			return
		}
	}

	serviceMode := p.getServiceMode(service.Annotations)

	for id, sp := range service.Spec.Ports {
		key := buildKey(service.Name, service.Namespace, sp.Port)

		if serviceMode == k8s.ServiceTypeHTTP {
			config.HTTP.Services[key] = p.buildService(endpoints)
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
		config.TCP.Services[key] = p.buildTCPService(endpoints)
	}
}

func (p *Provider) deleteServiceFromConfig(service *corev1.Service, config *dynamic.Configuration) {
	serviceMode := p.getServiceMode(service.Annotations)

	for _, sp := range service.Spec.Ports {
		key := buildKey(service.Name, service.Namespace, sp.Port)

		if serviceMode == k8s.ServiceTypeHTTP {
			delete(config.HTTP.Routers, key)
			delete(config.HTTP.Services, key)
			delete(config.HTTP.Middlewares, key)
			continue
		}

		delete(config.TCP.Routers, key)
		delete(config.TCP.Services, key)
	}
}

func (p *Provider) getServiceMode(annotations map[string]string) string {
	mode := annotations[k8s.AnnotationServiceType]

	if mode == "" {
		return p.defaultMode
	}
	return mode
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
