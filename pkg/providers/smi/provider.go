package smi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/providers/base"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	access "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specs "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	split "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha2"
	accessLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specsLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"
)

// Provider holds a client to access the provider.
type Provider struct {
	defaultMode          string
	tcpStateTable        *k8s.State
	ignored              k8s.IgnoreWrapper
	serviceLister        listers.ServiceLister
	endpointsLister      listers.EndpointsLister
	podLister            listers.PodLister
	trafficTargetLister  accessLister.TrafficTargetLister
	httpRouteGroupLister specsLister.HTTPRouteGroupLister
	tcpRouteLister       specsLister.TCPRouteLister
	trafficSplitLister   splitLister.TrafficSplitLister
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
func New(defaultMode string, tcpStateTable *k8s.State, ignored k8s.IgnoreWrapper,
	serviceLister listers.ServiceLister,
	endpointsLister listers.EndpointsLister,
	podLister listers.PodLister,
	trafficTargetLister accessLister.TrafficTargetLister,
	httpRouteGroupLister specsLister.HTTPRouteGroupLister,
	tcpRouteLister specsLister.TCPRouteLister,
	trafficSplitLister splitLister.TrafficSplitLister) *Provider {
	p := &Provider{
		defaultMode:          defaultMode,
		tcpStateTable:        tcpStateTable,
		ignored:              ignored,
		serviceLister:        serviceLister,
		endpointsLister:      endpointsLister,
		podLister:            podLister,
		trafficTargetLister:  trafficTargetLister,
		httpRouteGroupLister: httpRouteGroupLister,
		tcpRouteLister:       tcpRouteLister,
		trafficSplitLister:   trafficSplitLister,
	}

	p.Init()

	return p
}

// BuildConfig builds the configuration for routing
// from a native kubernetes environment.
func (p *Provider) BuildConfig() (*dynamic.Configuration, error) {
	config := base.CreateBaseConfigWithReadiness()
	base.AddBaseSMIMiddlewares(config)

	services, err := p.serviceLister.Services(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to get services: %v", err)
	}

	endpoints, err := p.endpointsLister.Endpoints(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoints: %v", err)
	}

	trafficTargets, err := p.trafficTargetLister.TrafficTargets(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to get traffictargets: %v", err)
	}

	trafficSplits, err := p.trafficSplitLister.TrafficSplits(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to get trafficsplits: %v", err)
	}

	for _, service := range services {
		if p.ignored.IsIgnored(service.ObjectMeta) {
			continue
		}

		serviceMode := p.getServiceMode(service.Annotations[k8s.AnnotationServiceType])
		// Get all traffic targets in the service's namespace.
		trafficTargetsInNamespace := p.getTrafficTargetsWithDestinationInNamespace(service.Namespace, trafficTargets)
		log.Debugf("Found traffictargets for service %s/%s: %+v", service.Namespace, service.Name, trafficTargets)
		// Find all traffic targets that are applicable to the service in question.
		applicableTrafficTargets := p.getApplicableTrafficTargets(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), trafficTargetsInNamespace)
		log.Debugf("Found applicable traffictargets for service %s/%s: %+v", service.Namespace, service.Name, applicableTrafficTargets)
		// Group the traffic targets by destination, so that they can be built separately.
		groupedByDestinationTrafficTargets := p.groupTrafficTargetsByDestination(applicableTrafficTargets)
		log.Debugf("Found grouped traffictargets for service %s/%s: %+v", service.Namespace, service.Name, groupedByDestinationTrafficTargets)

		// Get all traffic split in the service's namespace.
		trafficSplitsInNamespace := p.getTrafficSplitsWithDestinationInNamespace(service.Namespace, trafficSplits)
		log.Debugf("Found trafficsplits for service %s/%s: %+v", service.Namespace, service.Name, trafficSplitsInNamespace)

		for _, groupedTrafficTargets := range groupedByDestinationTrafficTargets {
			for _, groupedTrafficTarget := range groupedTrafficTargets {
				for id, sp := range service.Spec.Ports {
					key := buildKey(service.Name, service.Namespace, sp.Port, groupedTrafficTarget.Name, groupedTrafficTarget.Namespace)

					//	For each source in the trafficTarget, get a list of IPs to whitelist.
					sourceIPs := p.getSourceIPFromSourceSlice(groupedTrafficTarget.Sources)
					whitelistKey := groupedTrafficTarget.Name + "-" + groupedTrafficTarget.Namespace + "-" + key + "-whitelist"
					whitelistMiddleware := k8s.BlockAllMiddlewareKey

					switch serviceMode {
					case k8s.ServiceTypeHTTP:
						if len(sourceIPs) > 0 {
							config.HTTP.Middlewares[whitelistKey] = createWhitelistMiddleware(sourceIPs)
							whitelistMiddleware = whitelistKey
						}

						scheme := base.GetScheme(service.Annotations)

						trafficSplit := base.GetTrafficSplitFromList(service.Name, trafficSplitsInNamespace)
						if trafficSplit == nil {
							config.HTTP.Routers[key] = p.buildHTTPRouterFromTrafficTarget(service.Name, service.Namespace, service.Spec.ClusterIP, groupedTrafficTarget, 5000+id, key, whitelistMiddleware)
							config.HTTP.Services[key] = p.buildHTTPServiceFromTrafficTarget(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), groupedTrafficTarget, scheme)

							continue
						}

						p.buildTrafficSplit(config, trafficSplit, sp, id, groupedTrafficTarget, whitelistMiddleware, scheme)
					case k8s.ServiceTypeTCP:
						meshPort := p.getMeshPort(service.Name, service.Namespace, sp.Port)
						config.TCP.Routers[key] = p.buildTCPRouterFromTrafficTarget(groupedTrafficTarget, meshPort, key)
						config.TCP.Services[key] = p.buildTCPServiceFromTrafficTarget(base.GetEndpointsFromList(service.Name, service.Namespace, endpoints), groupedTrafficTarget)
					}
				}
			}
		}
	}

	return config, nil
}

func (p *Provider) getSourceIPFromSourceSlice(sources []access.IdentityBindingSubject) []string {
	var result []string

	for _, source := range sources {
		// Get all pods in the associated source namespace.
		podList, err := p.podLister.Pods(source.Namespace).List(labels.Everything())
		if err != nil {
			log.Errorf("Could not list pods: %v", err)
			continue
		}

		for _, pod := range podList {
			if pod.Spec.ServiceAccountName != source.Name {
				// Pod does not have the correct ServiceAccountName
				continue
			}
			// Retrieve a list of sourceIPs from the list of pods.
			if pod.Status.PodIP != "" {
				result = append(result, pod.Status.PodIP)
			}
		}
	}

	return result
}

func (p *Provider) getTrafficTargetsWithDestinationInNamespace(namespace string, trafficTargets []*access.TrafficTarget) []*access.TrafficTarget {
	var result []*access.TrafficTarget

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

func (p *Provider) getTrafficSplitsWithDestinationInNamespace(namespace string, trafficSplits []*split.TrafficSplit) []*split.TrafficSplit {
	var result []*split.TrafficSplit

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

func (p *Provider) getApplicableTrafficTargets(endpoints *corev1.Endpoints, trafficTargets []*access.TrafficTarget) []*access.TrafficTarget {
	var result []*access.TrafficTarget

	if endpoints == nil {
		log.Debugf("No applicable TrafficTargets: no endpoint")
		return nil
	}

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

			if !p.validPodFound(subset.Addresses, trafficTarget.Destination.Name) {
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

func (p *Provider) validPodFound(addresses []corev1.EndpointAddress, destinationName string) bool {
	for _, address := range addresses {
		if address.TargetRef == nil {
			log.Error("Address has no target reference")
			continue
		}

		pod, err := p.podLister.Pods(address.TargetRef.Namespace).Get(address.TargetRef.Name)
		if err != nil {
			log.Errorf("Could not get pod %s/%s: %v", address.TargetRef.Namespace, address.TargetRef.Name, err)
			continue
		}

		if pod.Spec.ServiceAccountName == destinationName {
			return true
		}
	}

	return false
}

func (p *Provider) groupTrafficTargetsByDestination(trafficTargets []*access.TrafficTarget) map[destinationKey][]*access.TrafficTarget {
	result := make(map[destinationKey][]*access.TrafficTarget)

	for _, trafficTarget := range trafficTargets {
		t := trafficTarget.DeepCopy()
		key := destinationKey{
			name:      trafficTarget.Destination.Name,
			namespace: trafficTarget.Destination.Namespace,
			port:      trafficTarget.Destination.Port,
		}

		if _, ok := result[key]; !ok {
			// If the destination key does not exist, create the key.
			result[key] = []*access.TrafficTarget{}
		}

		result[key] = append(result[key], t)
	}

	return result
}

func (p *Provider) buildHTTPRouterFromTrafficTarget(serviceName, serviceNamespace, serviceIP string, trafficTarget *access.TrafficTarget, port int, key, middleware string) *dynamic.Router {
	var rule []string

	for _, spec := range trafficTarget.Specs {
		var builtRule []string

		if spec.Kind != "HTTPRouteGroup" {
			continue
		}

		rawHTTPRouteGroup, err := p.httpRouteGroupLister.HTTPRouteGroups(trafficTarget.Namespace).Get(spec.Name)
		if err != nil {
			log.Errorf("Error getting the HTTPRouteGroups %s in the same namespace %s as the TrafficTarget: %v", spec.Name, trafficTarget.Namespace, err)
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

func (p *Provider) buildTCPRouterFromTrafficTarget(trafficTarget *access.TrafficTarget, port int, key string) *dynamic.TCPRouter {
	var rule string

	for _, spec := range trafficTarget.Specs {
		if spec.Kind != "TCPRoute" {
			continue
		}

		_, err := p.tcpRouteLister.TCPRoutes(trafficTarget.Namespace).Get(spec.Name)
		if err != nil {
			log.Errorf("Error getting the TCPRoute %s in the same namespace %s as the TrafficTarget: %v", spec.Name, trafficTarget.Namespace, err)
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

func (p *Provider) buildRuleSnippetFromServiceAndMatch(name, namespace, ip string, match specs.HTTPMatch) string {
	var result []string

	if len(match.PathRegex) > 0 {
		preparedPath := match.PathRegex

		if strings.HasPrefix(match.PathRegex, "/") {
			preparedPath = strings.TrimPrefix(preparedPath, "/")
		}

		result = append(result, fmt.Sprintf("PathPrefix(`/{path:%s}`)", preparedPath))
	}

	if len(match.Methods) > 0 && match.Methods[0] != "*" {
		methods := strings.Join(match.Methods, "`,`")
		result = append(result, fmt.Sprintf("Method(`%s`)", methods))
	}

	result = append(result, fmt.Sprintf("(Host(`%s.%s.maesh`) || Host(`%s`))", name, namespace, ip))

	return strings.Join(result, " && ")
}

func (p *Provider) buildHTTPServiceFromTrafficTarget(endpoints *corev1.Endpoints, trafficTarget *access.TrafficTarget, scheme string) *dynamic.Service {
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
				pod, err := p.podLister.Pods(address.TargetRef.Namespace).Get(address.TargetRef.Name)
				if err != nil {
					log.Errorf("Could not get pod %s/%s: %v", address.TargetRef.Namespace, address.TargetRef.Name, err)
					continue
				}

				if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
					server := dynamic.Server{
						URL: fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(address.IP, strconv.FormatInt(int64(endpointPort.Port), 10))),
					}
					servers = append(servers, server)
				}
			}
		}
	}

	return &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			PassHostHeader: base.Bool(true),
			Servers:        servers,
		},
	}
}

func (p *Provider) buildTCPServiceFromTrafficTarget(endpoints *corev1.Endpoints, trafficTarget *access.TrafficTarget) *dynamic.TCPService {
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
				pod, err := p.podLister.Pods(address.TargetRef.Namespace).Get(address.TargetRef.Name)
				if err != nil {
					log.Errorf("Could not get pod %s/%s: %v", address.TargetRef.Namespace, address.TargetRef.Name, err)
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

func (p *Provider) buildTrafficSplit(config *dynamic.Configuration, trafficSplit *split.TrafficSplit,
	sp corev1.ServicePort, id int, trafficTarget *access.TrafficTarget, whitelistMiddleware string, scheme string) {
	var WRRServices []dynamic.WRRService

	for _, backend := range trafficSplit.Spec.Backends {
		endpoints, err := p.endpointsLister.Endpoints(trafficSplit.Namespace).Get(backend.Service)
		if err != nil {
			log.Errorf("Could not get endpoints for service %s/%s: %v", trafficSplit.Namespace, backend.Service, err)
			return
		}

		splitKey := buildKey(backend.Service, trafficSplit.Namespace, sp.Port, trafficTarget.Name, trafficTarget.Namespace)
		config.HTTP.Services[splitKey] = p.buildHTTPServiceFromTrafficTarget(endpoints, trafficTarget, scheme)

		WRRServices = append(WRRServices, dynamic.WRRService{
			Name:   splitKey,
			Weight: intToP(int64(backend.Weight)),
		})
	}

	svc, err := p.serviceLister.Services(trafficSplit.Namespace).Get(trafficSplit.Spec.Service)
	if err != nil {
		log.Errorf("Could not get service for service %s/%s: %v", trafficSplit.Namespace, trafficSplit.Spec.Service, err)
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
