package kubernetes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/message"
	"github.com/containous/traefik/pkg/config"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	client      k8s.CoreV1Client
	defaultMode string
}

// Init the provider.
func (p *Provider) Init() {

}

// New creates a new provider.
func New(client k8s.CoreV1Client, defaultMode string) *Provider {
	p := &Provider{
		client:      client,
		defaultMode: defaultMode,
	}

	p.Init()

	return p
}

// BuildConfiguration builds the configuration for routing
// from a native kubernetes environment.
func (p *Provider) BuildConfiguration(event message.Message, traefikConfig *config.Configuration) {
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

func (p *Provider) buildRouter(name, namespace, ip string, port int, serviceName string) *config.Router {
	return &config.Router{
		Rule:        fmt.Sprintf("Host(`%s.%s.traefik.mesh`) || Host(`%s`)", name, namespace, ip),
		EntryPoints: []string{fmt.Sprintf("ingress-%d", port)},
		Service:     serviceName,
	}
}

func (p *Provider) buildTCPRouter(port int, serviceName string) *config.TCPRouter {
	return &config.TCPRouter{
		Rule:        "HostSNI(`*`)",
		EntryPoints: []string{fmt.Sprintf("ingress-%d", port)},
		Service:     serviceName,
	}
}

func (p *Provider) buildService(endpoints *corev1.Endpoints) *config.Service {
	var servers []config.Server
	for _, subset := range endpoints.Subsets {
		for _, endpointPort := range subset.Ports {
			for _, address := range subset.Addresses {
				server := config.Server{
					URL: "http://" + net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10)),
				}
				servers = append(servers, server)
			}
		}
	}

	lb := &config.LoadBalancerService{
		PassHostHeader: true,
		Servers:        servers,
	}

	return &config.Service{
		LoadBalancer: lb,
	}
}

func (p *Provider) buildTCPService(endpoints *corev1.Endpoints) *config.TCPService {
	var servers []config.TCPServer
	for _, subset := range endpoints.Subsets {
		for _, endpointPort := range subset.Ports {
			for _, address := range subset.Addresses {
				server := config.TCPServer{
					Address: net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10)),
				}
				servers = append(servers, server)
			}
		}
	}

	lb := &config.TCPLoadBalancerService{
		Servers: servers,
	}

	return &config.TCPService{
		LoadBalancer: lb,
	}
}

func (p *Provider) buildServiceIntoConfig(service *corev1.Service, endpoints *corev1.Endpoints, config *config.Configuration) {
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

	serviceMode := p.getServiceMode(service.Annotations[k8s.AnnotationServiceType])

	for id, sp := range service.Spec.Ports {
		key := buildKey(service.Name, service.Namespace, sp.Port)

		if serviceMode == k8s.ServiceTypeHTTP {
			config.HTTP.Routers[key] = p.buildRouter(service.Name, service.Namespace, service.Spec.ClusterIP, 5000+id, key)
			config.HTTP.Services[key] = p.buildService(endpoints)
			continue
		}

		config.TCP.Routers[key] = p.buildTCPRouter(5000+id, key)
		config.TCP.Services[key] = p.buildTCPService(endpoints)
	}
}

func (p *Provider) deleteServiceFromConfig(service *corev1.Service, config *config.Configuration) {

	serviceMode := p.getServiceMode(service.Annotations[k8s.AnnotationServiceType])

	for _, sp := range service.Spec.Ports {
		key := buildKey(service.Name, service.Namespace, sp.Port)

		if serviceMode == k8s.ServiceTypeHTTP {
			delete(config.HTTP.Routers, key)
			delete(config.HTTP.Services, key)
			continue
		}

		delete(config.TCP.Routers, key)
		delete(config.TCP.Services, key)
	}
}

func (p *Provider) getServiceMode(mode string) string {
	if mode == "" {
		return p.defaultMode
	}
	return mode
}

func buildKey(name, namespace string, port int32) string {
	// Use the hash of the servicename.namespace.port as the key
	// So that we can update services based on their name
	// and not have to worry about duplicates on merges.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s.%s.%d", name, namespace, port)))
	dst := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(dst, sum[:])
	return string(dst)
}
