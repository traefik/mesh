package kubernetes

import (
	"fmt"
	"net"
	"strconv"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/config"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	client k8s.CoreV1Client
}

// Init the provider.
func (p *Provider) Init() error {
	return nil
}

// New creates a new provider.
func New(client k8s.CoreV1Client) *Provider {
	p := &Provider{
		client: client,
	}

	if err := p.Init(); err != nil {
		log.Errorln("Could not initialize Kubernetes Provider")
	}

	return p
}

// BuildConfiguration builds the configuration for routing
// from a native kubernetes environment.
func (p *Provider) BuildConfiguration() *config.Configuration {
	configRouters := make(map[string]*config.Router)
	configServices := make(map[string]*config.Service)
	namespaces, err := p.client.GetNamespaces()
	if err != nil {
		log.Error("Could not get a list of all namespaces")
	}

	for _, namespace := range namespaces {
		services, err := p.client.GetServices(namespace.Name)
		if err != nil {
			log.Errorf("Could not get a list of all services in namespace: %s", namespace.Name)
		}

		for _, service := range services {
			key := uuid.New().String()
			configRouters[key] = p.buildRouterFromService(service)
			configServices[key] = p.buildServiceFromService(service)

		}
	}

	return &config.Configuration{
		HTTP: &config.HTTPConfiguration{
			Routers:  configRouters,
			Services: configServices,
		},
	}
}

func (p *Provider) buildRouterFromService(service *corev1.Service) *config.Router {
	return &config.Router{
		Rule: fmt.Sprintf("Host(`%s.%s.traefik.mesh`) || Host(`%s`)", service.Name, service.Namespace, service.Spec.ClusterIP),
	}
}

func (p *Provider) buildServiceFromService(service *corev1.Service) *config.Service {
	var servers []config.Server

	endpoint, err := p.client.GetEndpoints(service.Namespace, service.Name)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil
	}
	for _, subset := range endpoint.Subsets {
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
