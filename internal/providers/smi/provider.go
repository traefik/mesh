package smi

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/config"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	client *k8s.ClientWrapper
}

// destinationKey is used to key a grouped map of trafficTargets.
type destinationKey struct {
	name      string
	namespace string
	port      string
}

// Init the provider.
func (p *Provider) Init() error {
	return nil
}

// New creates a new provider.
func New(client *k8s.ClientWrapper) *Provider {
	p := &Provider{
		client: client,
	}

	if err := p.Init(); err != nil {
		log.Errorln("Could not initialize SMI Provider")
	}

	return p
}

// BuildConfiguration builds the configuration for routing
// from a kubernetes environment, with SMI objects in play.
func (p *Provider) BuildConfiguration() *config.Configuration {
	configRouters := make(map[string]*config.Router)
	configServices := make(map[string]*config.Service)
	namespaces, err := p.client.GetNamespaces()
	if err != nil {
		log.Error("Could not get a list of all namespaces")
	}

	for _, namespace := range namespaces {
		trafficTargets := p.getTrafficTargetsWithDestinationInNamespace(namespace.Name)

		services, err := p.client.GetServices(namespace.Name)
		if err != nil {
			log.Errorf("Could not get a list of all services in namespace: %s", namespace.Name)
		}

		for _, service := range services {
			applicableTrafficTargets := p.getApplicableTrafficTargets(service, trafficTargets)

			groupedByDestinationTrafficTargets := p.groupTrafficTargetsByDestination(applicableTrafficTargets)

			for _, groupedTrafficTargets := range groupedByDestinationTrafficTargets {
				for _, groupedTrafficTarget := range groupedTrafficTargets {
					key := uuid.New().String()
					configRouters[key] = p.buildRouterFromTrafficTarget(service, groupedTrafficTarget)
					configServices[key] = p.buildServiceFromTrafficTarget(service, groupedTrafficTarget)
				}
			}

		}
	}

	return &config.Configuration{
		HTTP: &config.HTTPConfiguration{
			Routers:  configRouters,
			Services: configServices,
		},
	}
}

func (p *Provider) getTrafficTargetsWithDestinationInNamespace(namespace string) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget
	allTrafficTargets, err := p.client.GetTrafficTargets()
	if err != nil {
		log.Error("Could not get a list of all TrafficTargets")
	}

	for _, trafficTarget := range allTrafficTargets {
		if trafficTarget.Destination.Namespace != namespace {
			continue
		}
		result = append(result, trafficTarget)
	}

	return result
}

func (p *Provider) getApplicableTrafficTargets(service *corev1.Service, trafficTargets []*accessv1alpha1.TrafficTarget) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget

	endpoint, err := p.client.GetEndpoints(service.Namespace, service.Name)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil
	}

	for _, subset := range endpoint.Subsets {
		for _, trafficTarget := range trafficTargets {
			if service.Namespace != trafficTarget.Destination.Namespace {
				// Destination not in service namespace, skip.
				continue
			}

			var subsetMatch bool
			for _, endpointPort := range subset.Ports {
				if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port {
					subsetMatch = true
					break
				}
			}

			if !subsetMatch {
				// No subset port match on destination port, so subset is not affected
				continue
			}

			var validPodFound bool
			for _, address := range subset.Addresses {
				if pod, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name); err != nil {
					if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
						validPodFound = true
						break
					}
				}
			}

			if !validPodFound {
				// No valid pods with serviceAccound found on the subset, so it is not affected
				continue
			}

			// We have a subset match, and valid referenced pods for the trafficTarget.
			result = append(result, trafficTarget)
		}

	}

	return result
}

func (p *Provider) groupTrafficTargetsByDestination(trafficTargets []*accessv1alpha1.TrafficTarget) map[destinationKey][]*accessv1alpha1.TrafficTarget {
	result := make(map[destinationKey][]*accessv1alpha1.TrafficTarget)

	for _, trafficTarget := range trafficTargets {
		key := destinationKey{
			name:      trafficTarget.Destination.Name,
			namespace: trafficTarget.Destination.Namespace,
			port:      trafficTarget.Destination.Port,
		}

		if _, ok := result[key]; !ok {
			// If the destination key does not exist, create the key.
			result[key] = []*accessv1alpha1.TrafficTarget{}
		}

		result[key] = append(result[key], trafficTarget)
	}

	return result
}

func (p *Provider) buildRouterFromTrafficTarget(service *corev1.Service, trafficTarget *accessv1alpha1.TrafficTarget) *config.Router {
	var rule []string
	for _, spec := range trafficTarget.Specs {
		if spec.Kind != "HTTPRouteGroup" {
			// TCP is unsupported for now.
			continue
		}
		var builtRule []string
		rawHTTPRouteGroup, err := p.client.GetHTTPRouteGroup(trafficTarget.Namespace, spec.Name)
		if err != nil {
			log.Errorf("Error getting HTTPRouteGroup: %v", err)
			continue
		}

		for _, match := range spec.Matches {
			for _, httpMatch := range rawHTTPRouteGroup.Matches {
				if match != httpMatch.Name {
					// Matches specified, add only matches from route group
					continue
				}
				builtRule = append(builtRule, p.buildRuleSnippetFromServiceAndMatch(service, httpMatch))
			}
		}
		rule = append(rule, "("+strings.Join(builtRule, " || ")+")")
	}

	return &config.Router{
		Rule: strings.Join(rule, " || "),
	}
}

func (p *Provider) buildRuleSnippetFromServiceAndMatch(service *corev1.Service, match specsv1alpha1.HTTPMatch) string {
	var result []string
	if len(match.PathRegex) > 0 {
		result = append(result, fmt.Sprintf("PathPrefix(`%s`)", match.PathRegex))
	}

	if len(match.Methods) > 0 {
		methods := strings.Join(match.Methods, ",")
		result = append(result, fmt.Sprintf("Methods(%s)", methods))
	}

	result = append(result, fmt.Sprintf("Host(`%s.%s.traefik.mesh`) || Host(`%s`)", service.Name, service.Namespace, service.Spec.ClusterIP))

	return "(" + strings.Join(result, " && ") + ")"
}

func (p *Provider) buildServiceFromTrafficTarget(service *corev1.Service, trafficTarget *accessv1alpha1.TrafficTarget) *config.Service {
	var servers []config.Server

	if service.Namespace != trafficTarget.Destination.Namespace {
		// Destination not in service namespace log error.
		log.Errorf("TrafficTarget %s/%s destination not in namespace %s", trafficTarget.Namespace, trafficTarget.Name, service.Namespace)
		return nil
	}

	endpoint, err := p.client.GetEndpoints(service.Namespace, service.Name)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil
	}
	for _, subset := range endpoint.Subsets {
		var subsetMatch bool
		for _, endpointPort := range subset.Ports {
			if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port {
				subsetMatch = true
				break
			}
		}

		if !subsetMatch {
			// No subset port match on destination port, so subset is not affected
			continue
		}

		for _, address := range subset.Addresses {
			if pod, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name); err != nil {
				if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
					server := config.Server{
						URL: "http://" + net.JoinHostPort(address.IP, trafficTarget.Destination.Port),
					}
					servers = append(servers, server)
				}
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
