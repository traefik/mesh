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
	client k8s.CoreV1Client
}

// Init the provider.
func (p *Provider) Init() {

}

// New creates a new provider.
func New(client k8s.CoreV1Client) *Provider {
	p := &Provider{
		client: client,
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
			p.addServiceToConfig(obj, traefikConfig)
		case message.TypeUpdated:

		case message.TypeDeleted:
		}
	case *corev1.Endpoints:
	}

}

func (p *Provider) buildRouter(name, namespace, ip string, port int, serviceName string) *config.Router {
	return &config.Router{
		Rule:        fmt.Sprintf("Host(`%s.%s.traefik.mesh`) || Host(`%s`)", name, namespace, ip),
		EntryPoints: []string{fmt.Sprintf("ingress-%d", port)},
		Service:     serviceName,
	}
}

func (p *Provider) buildService(name, namespace string) *config.Service {
	var servers []config.Server
	endpoint, exists, err := p.client.GetEndpoints(namespace, name)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", namespace, name, err)
		return nil
	}
	if !exists {
		log.Errorf("endpoints for service %s/%s do not exist", namespace, name)
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

func (p *Provider) addServiceToConfig(service *corev1.Service, config *config.Configuration) {
	for id, sp := range service.Spec.Ports {

		// Use the hash of the servicename.namespace.port as the key
		// So that we can update services based on their name
		// and not have to worry about duplicates on merges.
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s.%s.%d", service.Name, service.Namespace, sp.Port)))
		dst := make([]byte, hex.EncodedLen(len(sum)))
		hex.Encode(dst, sum[:])
		key := string(dst)

		config.HTTP.Routers[key] = p.buildRouter(service.Name, service.Namespace, service.Spec.ClusterIP, 5000+id, key)
		config.HTTP.Services[key] = p.buildService(service.Name, service.Namespace)
	}
}
