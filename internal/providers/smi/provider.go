package smi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/message"
	"github.com/containous/traefik/pkg/config"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	client      k8s.Client
	defaultMode string
}

// destinationKey is used to key a grouped map of trafficTargets.
type destinationKey struct {
	name      string
	namespace string
	port      string
}

// Init the provider.
func (p *Provider) Init() {
}

// New creates a new provider.
func New(client k8s.Client, defaultMode string) *Provider {
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
			//FIXME: Implement
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
	// Get all traffic targets in the service's namespace.
	trafficTargets := p.getTrafficTargetsWithDestinationInNamespace(service.Namespace)
	// Find all traffic targets that are applicable to the service in question.
	applicableTrafficTargets := p.getApplicableTrafficTargets(service.Name, service.Namespace, trafficTargets)
	// Group the traffic targets by destination, so that they can be built separately.
	groupedByDestinationTrafficTargets := p.groupTrafficTargetsByDestination(applicableTrafficTargets)

	for _, groupedTrafficTargets := range groupedByDestinationTrafficTargets {
		for _, groupedTrafficTarget := range groupedTrafficTargets {
			for id, sp := range service.Spec.Ports {
				key := buildKey(service.Name, service.Namespace, sp.Port, groupedTrafficTarget.Name, groupedTrafficTarget.Namespace)

				if serviceMode == k8s.ServiceTypeHTTP {
					config.HTTP.Routers[key] = p.buildRouterFromTrafficTarget(service.Name, service.Namespace, service.Spec.ClusterIP, groupedTrafficTarget, 5000+id, key)
					config.HTTP.Services[key] = p.buildServiceFromTrafficTarget(endpoints, groupedTrafficTarget)
					continue
				}
				// FIXME: Implement TCP routes
			}
		}
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

func (p *Provider) getApplicableTrafficTargets(serviceName, serviceNamespace string, trafficTargets []*accessv1alpha1.TrafficTarget) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget

	endpoint, exists, err := p.client.GetEndpoints(serviceName, serviceNamespace)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", serviceName, serviceNamespace, err)
		return nil
	}
	if !exists {
		log.Errorf("endpoints for service %s/%s do not exist", serviceName, serviceNamespace)
		return nil
	}
	for _, subset := range endpoint.Subsets {
		for _, trafficTarget := range trafficTargets {
			if serviceNamespace != trafficTarget.Destination.Namespace {
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
				if pod, exists, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name); err != nil {
					if exists {
						if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
							validPodFound = true
							break
						}
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

func (p *Provider) buildRouterFromTrafficTarget(serviceName, serviceNamespace, serviceIP string, trafficTarget *accessv1alpha1.TrafficTarget, port int, key string) *config.Router {
	var rule []string
	for _, spec := range trafficTarget.Specs {
		if spec.Kind != "HTTPRouteGroup" {
			log.Warn("TCP is unsupported for now.")
			continue
		}
		var builtRule []string
		rawHTTPRouteGroup, exists, err := p.client.GetHTTPRouteGroup(trafficTarget.Namespace, spec.Name)
		if err != nil {
			log.Errorf("Error getting HTTPRouteGroup: %v", err)
			continue
		}
		if !exists {
			log.Errorf("HTTPRouteGroup %s/%s does not exist", trafficTarget.Namespace, spec.Name)
			continue
		}
		for _, match := range spec.Matches {
			for _, httpMatch := range rawHTTPRouteGroup.Matches {
				if match != httpMatch.Name {
					// Matches specified, add only matches from route group
					continue
				}
				builtRule = append(builtRule, p.buildRuleSnippetFromServiceAndMatch(serviceName, serviceNamespace, serviceIP, httpMatch))
			}
		}
		rule = append(rule, "("+strings.Join(builtRule, " || ")+")")
	}

	return &config.Router{
		Rule:        strings.Join(rule, " || "),
		EntryPoints: []string{fmt.Sprintf("ingress-%d", port)},
		Service:     key,
	}
}

func (p *Provider) buildRuleSnippetFromServiceAndMatch(name, namespace, ip string, match specsv1alpha1.HTTPMatch) string {
	var result []string
	if len(match.PathRegex) > 0 {
		result = append(result, fmt.Sprintf("PathPrefix(`%s`)", match.PathRegex))
	}

	if len(match.Methods) > 0 {
		methods := strings.Join(match.Methods, ",")
		result = append(result, fmt.Sprintf("Methods(%s)", methods))
	}

	result = append(result, fmt.Sprintf("(Host(`%s.%s.traefik.mesh`) || Host(`%s`))", name, namespace, ip))

	return "(" + strings.Join(result, " && ") + ")"
}

func (p *Provider) buildServiceFromTrafficTarget(endpoints *corev1.Endpoints, trafficTarget *accessv1alpha1.TrafficTarget) *config.Service {
	var servers []config.Server

	if endpoints.Namespace != trafficTarget.Destination.Namespace {
		// Destination not in service namespace log error.
		log.Errorf("TrafficTarget %s/%s destination not in namespace %s", trafficTarget.Namespace, trafficTarget.Name, endpoints.Namespace)
		return nil
	}

	for _, subset := range endpoints.Subsets {
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
			if pod, exists, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name); err != nil {
				if exists {
					if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
						server := config.Server{
							URL: "http://" + net.JoinHostPort(address.IP, trafficTarget.Destination.Port),
						}
						servers = append(servers, server)
					}
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

func (p *Provider) getServiceMode(mode string) string {
	if mode == "" {
		return p.defaultMode
	}
	return mode
}

func buildKey(name, namespace string, port int32, ttname, ttnamespace string) string {
	// Use the hash of the servicename.namespace.port.traffictargetname.traffictargetnamespace as the key
	// So that we can update services based on their name
	// and not have to worry about duplicates on merges.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s.%s.%d.%s.%s", name, namespace, port, ttname, ttnamespace)))
	dst := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(dst, sum[:])
	return string(dst)
}
