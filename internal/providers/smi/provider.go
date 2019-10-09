package smi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/providers/base"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	splitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	client        k8s.Client
	defaultMode   string
	meshNamespace string
	tcpStateTable *k8s.State
	ignored       k8s.IgnoreWrapper
}

// destinationKey is used to key a grouped map of trafficTargets.
type destinationKey struct {
	name      string
	namespace string
	port      string
}

// Init the provider.
func (p *Provider) Init() {}

// New creates a new provider.
func New(client k8s.Client, defaultMode string, meshNamespace string, tcpStateTable *k8s.State, ignored k8s.IgnoreWrapper) *Provider {
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

// BuildConfig builds the configuration for routing
// from a native kubernetes environment.
func (p *Provider) BuildConfig() (*dynamic.Configuration, error) {
	config := base.CreateBaseConfigWithReadiness()
	base.AddBaseSMIMiddlewares(config)

	services, err := p.client.GetServices(metav1.NamespaceAll)
	if err != nil {
		return nil, fmt.Errorf("unable to get services: %v", err)
	}

	endpoints, err := p.client.GetEndpointses(metav1.NamespaceAll)
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoints: %v", err)
	}

	trafficTargets, err := p.client.GetTrafficTargets()
	if err != nil {
		return nil, fmt.Errorf("unable to get traffictargets: %v", err)
	}

	trafficSplits, err := p.client.GetTrafficSplits()
	if err != nil {
		return nil, fmt.Errorf("unable to get trafficsplits: %v", err)
	}

	for _, service := range services {
		if p.ignored.Ignored(service.Name, service.Namespace) {
			continue
		}

		serviceMode := p.getServiceMode(service.Annotations[k8s.AnnotationServiceType])
		// Get all traffic targets in the service's namespace.
		trafficTargetsInNamespace := p.getTrafficTargetsWithDestinationInNamespace(service.Namespace, trafficTargets)
		log.Debugf("Found traffictargets for service %s/%s: %+v\n", service.Namespace, service.Name, trafficTargets)
		// Find all traffic targets that are applicable to the service in question.
		applicableTrafficTargets := p.getApplicableTrafficTargets(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), trafficTargetsInNamespace)
		log.Debugf("Found applicable traffictargets for service %s/%s: %+v\n", service.Namespace, service.Name, applicableTrafficTargets)
		// Group the traffic targets by destination, so that they can be built separately.
		groupedByDestinationTrafficTargets := p.groupTrafficTargetsByDestination(applicableTrafficTargets)
		log.Debugf("Found grouped traffictargets for service %s/%s: %+v\n", service.Namespace, service.Name, groupedByDestinationTrafficTargets)

		// Get all traffic split in the service's namespace.
		trafficSplitsInNamespace := p.getTrafficSplitsWithDestinationInNamespace(service.Namespace, trafficSplits)
		log.Debugf("Found trafficsplits for service %s/%s: %+v\n", service.Namespace, service.Name, trafficSplitsInNamespace)

		for _, groupedTrafficTargets := range groupedByDestinationTrafficTargets {
			for _, groupedTrafficTarget := range groupedTrafficTargets {
				for id, sp := range service.Spec.Ports {
					key := buildKey(service.Name, service.Namespace, sp.Port, groupedTrafficTarget.Name, groupedTrafficTarget.Namespace)

					//	For each source in the trafficTarget, get a list of IPs to whitelist.
					var sourceIPs []string

					for _, source := range groupedTrafficTarget.Sources {
						fieldSelector := fmt.Sprintf("spec.serviceAccountName=%s", source.Name)
						// Get all pods with the associated source serviceAccount (can only be in the source namespaces).
						podList, err := p.client.ListPodWithOptions(source.Namespace, metav1.ListOptions{FieldSelector: fieldSelector})
						if err != nil {
							log.Errorf("Could not list pods: %v", err)
							continue
						}

						// Retrieve a list of sourceIPs from the list of pods.
						for _, pod := range podList.Items {
							if pod.Status.PodIP != "" {
								sourceIPs = append(sourceIPs, pod.Status.PodIP)
							}
						}
					}

					whitelistKey := groupedTrafficTarget.Name + "-" + groupedTrafficTarget.Namespace + "-" + key + "-whitelist"
					whitelistMiddleware := k8s.BlockAllMiddlewareKey

					if serviceMode == k8s.ServiceTypeHTTP {
						if len(sourceIPs) > 0 {
							config.HTTP.Middlewares[whitelistKey] = createWhitelistMiddleware(sourceIPs)
							whitelistMiddleware = whitelistKey
						}

						trafficSplit := base.GetTrafficSplitFromList(service.Name, trafficSplitsInNamespace)
						if trafficSplit == nil {
							config.HTTP.Routers[key] = p.buildHTTPRouterFromTrafficTarget(service.Name, service.Namespace, service.Spec.ClusterIP, groupedTrafficTarget, 5000+id, key, whitelistMiddleware)
							config.HTTP.Services[key] = p.buildHTTPServiceFromTrafficTarget(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), groupedTrafficTarget)
							continue
						}

						p.buildTrafficSplit(config, trafficSplit, sp, id, groupedTrafficTarget, whitelistMiddleware)
					}

					meshPort := p.getMeshPort(service.Name, service.Namespace, sp.Port)
					config.TCP.Routers[key] = p.buildTCPRouterFromTrafficTarget(service.Name, service.Namespace, service.Spec.ClusterIP, groupedTrafficTarget, meshPort, key)
					config.TCP.Services[key] = p.buildTCPServiceFromTrafficTarget(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), groupedTrafficTarget)
				}
			}
		}
	}

	return config, nil
}

func (p *Provider) getTrafficTargetsWithDestinationInNamespace(namespace string, trafficTargets []*accessv1alpha1.TrafficTarget) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget

	for _, trafficTarget := range trafficTargets {
		if trafficTarget.Destination.Namespace == namespace {
			result = append(result, trafficTarget)
		}
	}

	if len(result) == 0 {
		log.Debugf("No TrafficTargets with destination in namespace: %s", namespace)
	}

	return result
}

func (p *Provider) getTrafficSplitsWithDestinationInNamespace(namespace string, trafficSplits []*splitv1alpha1.TrafficSplit) []*splitv1alpha1.TrafficSplit {
	var result []*splitv1alpha1.TrafficSplit

	for _, trafficSplit := range trafficSplits {
		if trafficSplit.Namespace == namespace {
			result = append(result, trafficSplit)
		}
	}

	if len(result) == 0 {
		log.Debugf("No TrafficSplits in namespace: %s", namespace)
	}

	return result
}

func (p *Provider) getApplicableTrafficTargets(endpoints *corev1.Endpoints, trafficTargets []*accessv1alpha1.TrafficTarget) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget

	if len(endpoints.Subsets) == 0 {
		log.Debugf("No applicable TrafficTargets for service %s/%s: No endpoint subsets", endpoints.Namespace, endpoints.Name)
	}

	for _, subset := range endpoints.Subsets {
		for _, trafficTarget := range trafficTargets {
			if endpoints.Namespace != trafficTarget.Destination.Namespace {
				// Destination not in service namespace, skip.
				log.Debugf("Destination namespace for TrafficTarget: %s not in service namespace: %s", trafficTarget.Destination.Name, endpoints.Namespace)
				continue
			}

			var subsetMatch bool

			for _, endpointPort := range subset.Ports {
				if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port || trafficTarget.Destination.Port == "" {
					subsetMatch = true
					break
				}
			}

			if !subsetMatch {
				// No subset port match on destination port, so subset is not affected
				log.Debugf("TrafficTarget: %s does not match destination ports for endpoints %s/%s", trafficTarget.Destination.Name, endpoints.Namespace, endpoints.Name)
				continue
			}

			var validPodFound bool

			for _, address := range subset.Addresses {
				pod, exists, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name)
				if err != nil {
					log.Errorf("Could not get pod %s/%s: %v", address.TargetRef.Namespace, address.TargetRef.Name, err)
					continue
				}

				if !exists {
					log.Errorf("pod %s/%s do not exist", address.TargetRef.Namespace, address.TargetRef.Name)
					continue
				}

				if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
					validPodFound = true
					break
				}
			}

			if !validPodFound {
				// No valid pods with serviceAccount found on the subset, so it is not affected
				log.Debugf("Endpoints %s/%s has no valid pods with destination service account: %s", endpoints.Namespace, endpoints.Name, trafficTarget.Destination.Name)
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
		t := trafficTarget.DeepCopy()
		key := destinationKey{
			name:      trafficTarget.Destination.Name,
			namespace: trafficTarget.Destination.Namespace,
			port:      trafficTarget.Destination.Port,
		}

		if _, ok := result[key]; !ok {
			// If the destination key does not exist, create the key.
			result[key] = []*accessv1alpha1.TrafficTarget{}
		}

		result[key] = append(result[key], t)
	}

	return result
}

func (p *Provider) buildHTTPRouterFromTrafficTarget(serviceName, serviceNamespace, serviceIP string, trafficTarget *accessv1alpha1.TrafficTarget, port int, key, middleware string) *dynamic.Router {
	var rule []string

	for _, spec := range trafficTarget.Specs {
		var builtRule []string
		if spec.Kind != "HTTPRouteGroup" {
			continue
		}

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

	return &dynamic.Router{
		Rule:        strings.Join(rule, " || "),
		EntryPoints: []string{fmt.Sprintf("http-%d", port)},
		Service:     key,
		Middlewares: []string{middleware},
	}
}

func (p *Provider) buildTCPRouterFromTrafficTarget(serviceName, serviceNamespace, serviceIP string, trafficTarget *accessv1alpha1.TrafficTarget, port int, key string) *dynamic.TCPRouter {
	var rule string
	for _, spec := range trafficTarget.Specs {
		if spec.Kind != "TCPRoute" {
			continue
		}
		_, exists, err := p.client.GetTCPRoute(trafficTarget.Namespace, spec.Name)
		if err != nil {
			log.Errorf("Error getting TCPRoute: %v", err)
			continue
		}
		if !exists {
			log.Errorf("TCPRoute %s/%s does not exist", trafficTarget.Namespace, spec.Name)
			continue
		}
		rule = "HostSNI(`*`)"
	}

	return &dynamic.TCPRouter{
		Rule:        rule,
		EntryPoints: []string{fmt.Sprintf("tcp-%d", port)},
		Service:     key,
	}
}

func (p *Provider) buildHTTPRule(serviceName string, serviceNamespace string, serviceIP string, trafficTarget *accessv1alpha1.TrafficTarget, matches []string, specName string) []string {
	var builtRule []string
	rawHTTPRouteGroup, exists, err := p.client.GetHTTPRouteGroup(trafficTarget.Namespace, specName)
	if err != nil {
		log.Errorf("Error getting HTTPRouteGroup: %v", err)
		return builtRule
	}
	if !exists {
		log.Errorf("HTTPRouteGroup %s/%s does not exist", trafficTarget.Namespace, specName)
		return builtRule
	}
	for _, match := range matches {
		for _, httpMatch := range rawHTTPRouteGroup.Matches {
			if match != httpMatch.Name {
				// Matches specified, add only matches from route group
				continue
			}
			builtRule = append(builtRule, p.buildRuleSnippetFromServiceAndMatch(serviceName, serviceNamespace, serviceIP, httpMatch))
		}
	}
	return builtRule
}

func (p *Provider) buildTCPRule(serviceName string, serviceNamespace string, serviceIP string, trafficTarget *accessv1alpha1.TrafficTarget, matches []string, specName string) string {
	_, exists, err := p.client.GetTCPRoute(trafficTarget.Namespace, specName)
	if err != nil {
		log.Errorf("Error getting TCPRoute: %v", err)
		return ""
	}
	if !exists {
		log.Errorf("TCPRoute %s/%s does not exist", trafficTarget.Namespace, specName)
		return ""
	}
	return fmt.Sprintf("HostSNI(`*`)")
}

func (p *Provider) buildRuleSnippetFromServiceAndMatch(name, namespace, ip string, match specsv1alpha1.HTTPMatch) string {
	var result []string
	if len(match.PathRegex) > 0 {
		result = append(result, fmt.Sprintf("PathPrefix(`%s`)", match.PathRegex))
	}

	if len(match.Methods) > 0 && match.Methods[0] != "*" {
		methods := strings.Join(match.Methods, "`,`")
		result = append(result, fmt.Sprintf("Method(`%s`)", methods))
	}

	result = append(result, fmt.Sprintf("(Host(`%s.%s.%s`) || Host(`%s`))", name, namespace, p.meshNamespace, ip))

	return strings.Join(result, " && ")
}

func (p *Provider) buildHTTPServiceFromTrafficTarget(endpoints *corev1.Endpoints, trafficTarget *accessv1alpha1.TrafficTarget) *dynamic.Service {
	var servers []dynamic.Server

	if endpoints.Namespace != trafficTarget.Destination.Namespace {
		// Destination not in service namespace log error.
		log.Errorf("TrafficTarget %s/%s destination not in namespace %s", trafficTarget.Namespace, trafficTarget.Name, endpoints.Namespace)
		return nil
	}

	for _, subset := range endpoints.Subsets {
		var subsetMatch bool

		for _, endpointPort := range subset.Ports {
			if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port || trafficTarget.Destination.Port == "" {
				subsetMatch = true
				break
			}
		}

		if !subsetMatch {
			// No subset port match on destination port, so subset is not affected
			continue
		}

		for _, endpointPort := range subset.Ports {
			for _, address := range subset.Addresses {
				pod, exists, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name)
				if err != nil {
					log.Errorf("Could not get pod %s/%s: %v", address.TargetRef.Namespace, address.TargetRef.Name, err)
					continue
				}

				if !exists {
					log.Errorf("pod %s/%s do not exist", address.TargetRef.Namespace, address.TargetRef.Name)
					continue
				}

				if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
					server := dynamic.Server{
						URL: "http://" + net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10)),
					}
					servers = append(servers, server)
				}
			}
		}
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			PassHostHeader: true,
			Servers:        servers,
		},
	}
}

func (p *Provider) buildTCPServiceFromTrafficTarget(endpoints *corev1.Endpoints, trafficTarget *accessv1alpha1.TrafficTarget) *dynamic.TCPService {
	var servers []dynamic.TCPServer

	if endpoints.Namespace != trafficTarget.Destination.Namespace {
		// Destination not in service namespace log error.
		log.Errorf("TrafficTarget %s/%s destination not in namespace %s", trafficTarget.Namespace, trafficTarget.Name, endpoints.Namespace)
		return nil
	}

	for _, subset := range endpoints.Subsets {
		var subsetMatch bool
		for _, endpointPort := range subset.Ports {
			if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port || trafficTarget.Destination.Port == "" {
				subsetMatch = true
				break
			}
		}

		if !subsetMatch {
			// No subset port match on destination port, so subset is not affected
			continue
		}

		for _, endpointPort := range subset.Ports {
			for _, address := range subset.Addresses {
				pod, exists, err := p.client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name)
				if err != nil {
					log.Errorf("Could not get pod %s/%s: %v", address.TargetRef.Namespace, address.TargetRef.Name, err)
					continue
				}
				if !exists {
					log.Errorf("pod %s/%s do not exist", address.TargetRef.Namespace, address.TargetRef.Name)
					continue
				}
				if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
					server := dynamic.TCPServer{
						Address: net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10)),
					}
					servers = append(servers, server)
				}
			}
		}
	}

	return &dynamic.TCPService{
		LoadBalancer: &dynamic.TCPServersLoadBalancer{
			Servers: servers,
		},
	}
}

func (p *Provider) getServiceMode(mode string) string {
	if mode == "" {
		return p.defaultMode
	}

	return mode
}

func (p *Provider) buildTrafficSplit(config *dynamic.Configuration, trafficSplit *splitv1alpha1.TrafficSplit, sp corev1.ServicePort, id int, trafficTarget *accessv1alpha1.TrafficTarget, whitelistMiddleware string) {
	var WRRServices []dynamic.WRRService

	for _, backend := range trafficSplit.Spec.Backends {
		endpoints, exists, err := p.client.GetEndpoints(trafficSplit.Namespace, backend.Service)
		if err != nil {
			log.Errorf("Could not get endpoints for service %s/%s: %v", trafficSplit.Namespace, backend.Service, err)
			return
		}

		if !exists {
			log.Errorf("endpoints for service %s/%s do not exist", trafficSplit.Namespace, backend.Service)
			return
		}

		splitKey := buildKey(backend.Service, trafficSplit.Namespace, sp.Port, trafficTarget.Name, trafficTarget.Namespace)
		config.HTTP.Services[splitKey] = p.buildHTTPServiceFromTrafficTarget(endpoints, trafficTarget)
		WRRServices = append(WRRServices, dynamic.WRRService{
			Name:   splitKey,
			Weight: intToP(backend.Weight.Value()),
		})
	}

	svc, exists, err := p.client.GetService(trafficSplit.Namespace, trafficSplit.Spec.Service)
	if err != nil {
		log.Errorf("Could not get service for service %s/%s: %v", trafficSplit.Namespace, trafficSplit.Spec.Service, err)
		return
	}

	if !exists {
		log.Errorf("service %s/%s do not exist", trafficSplit.Namespace, trafficSplit.Spec.Service)
		return
	}

	svcWeighted := &dynamic.Service{
		Weighted: &dynamic.WeightedRoundRobin{
			Services: WRRServices,
		},
	}

	weightedKey := buildKey(svc.Name, svc.Namespace, sp.Port, trafficTarget.Name, trafficTarget.Namespace)
	config.HTTP.Routers[weightedKey] = p.buildHTTPRouterFromTrafficTarget(trafficSplit.Spec.Service, trafficSplit.Namespace, svc.Spec.ClusterIP, trafficTarget, 5000+id, weightedKey, whitelistMiddleware)
	config.HTTP.Services[weightedKey] = svcWeighted
}

func (p *Provider) getMeshPort(serviceName, serviceNamespace string, servicePort int32) int {
	for port, v := range p.tcpStateTable.Table {
		if v.Name == serviceName && v.Namespace == serviceNamespace && v.Port == servicePort {
			return port
		}
	}
	return 0
}

func buildKey(serviceName, namespace string, port int32, ttName, ttNamespace string) string {
	// Use the hash of the servicename.namespace.port.traffictargetname.traffictargetnamespace as the key
	// So that we can update services based on their name
	// and not have to worry about duplicates on merges.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s.%s.%d.%s.%s", serviceName, namespace, port, ttName, ttNamespace)))
	dst := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(dst, sum[:])
	fullHash := string(dst)

	return fmt.Sprintf("%.10s-%.10s-%d-%.10s-%.10s-%.16s", serviceName, namespace, port, ttName, ttNamespace, fullHash)
}

func createWhitelistMiddleware(sourceIPs []string) *dynamic.Middleware {
	// Create middleware.
	return &dynamic.Middleware{
		IPWhiteList: &dynamic.IPWhiteList{
			SourceRange: sourceIPs,
		},
	}
}

func intToP(v int64) *int {
	i := int(v)
	return &i
}
