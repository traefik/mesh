package topology

import (
	"fmt"
	"strconv"

	mk8s "github.com/containous/maesh/pkg/k8s"
	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha1"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha1"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	accessLister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specLister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitLister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"
)

// Builder builds Topology objects based on the current state of a kubernetes cluster.
type Builder struct {
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	PodLister            listers.PodLister
	TrafficTargetLister  accessLister.TrafficTargetLister
	TrafficSplitLister   splitLister.TrafficSplitLister
	HTTPRouteGroupLister specLister.HTTPRouteGroupLister
	TCPRoutesLister      specLister.TCPRouteLister
	Logger               logrus.FieldLogger
}

// Build builds a graph representing the possible interactions between Pods and Services based on the current state
// of the kubernetes cluster.
func (b *Builder) Build(ignoredResources mk8s.IgnoreWrapper) (*Topology, error) {
	topology := NewTopology()

	res, err := b.loadResources(ignoredResources)
	if err != nil {
		return nil, fmt.Errorf("unable to load resources: %w", err)
	}

	// Populate services.
	for _, svc := range res.Services {
		b.evaluateService(res, topology, svc)
	}

	// Populate services with traffic-target definitions.
	for _, tt := range res.TrafficTargets {
		if err := b.evaluateTrafficTarget(res, topology, tt); err != nil {
			b.Logger.Errorf("Unable to evaluate TrafficSplit %s/%s: %v", tt.Namespace, tt.Name, err)
		}
	}

	// Populate services with traffic-split definitions.
	for _, ts := range res.TrafficSplits {
		if err := b.evaluateTrafficSplit(topology, ts); err != nil {
			b.Logger.Errorf("Unable to evaluate TrafficSplit %s/%s: %v", ts.Namespace, ts.Name, err)
		}
	}

	b.populateTrafficSplitsAuthorizedIncomingTraffic(topology)

	return topology, nil
}

func (b *Builder) evaluateService(res *resources, topology *Topology, svc *v1.Service) {
	svcKey := Key{svc.Name, svc.Namespace}

	svcPods := res.PodsBySvc[svcKey]
	pods := make([]*Pod, len(svcPods))

	for i, pod := range svcPods {
		pods[i] = getOrCreatePod(topology, pod)
	}

	topology.Services[svcKey] = &Service{
		Name:        svc.Name,
		Namespace:   svc.Namespace,
		Selector:    svc.Spec.Selector,
		Annotations: svc.Annotations,
		Ports:       svc.Spec.Ports,
		ClusterIP:   svc.Spec.ClusterIP,
		Pods:        pods,
	}
}

// evaluateTrafficTarget evaluates the given traffic-target. It adds a ServiceTrafficTargets on every Service which
// has pods with a service-account being the one defined in the traffic-target destination.
// When a ServiceTrafficTarget gets added to a Service, each source and destination pod will be added to the topology
// and linked to it.
func (b *Builder) evaluateTrafficTarget(res *resources, topology *Topology, tt *access.TrafficTarget) error {
	destSaKey := Key{tt.Destination.Name, tt.Destination.Namespace}

	sources := b.buildTrafficTargetSources(res, topology, tt)

	specs, err := b.buildTrafficTargetSpecs(res, tt)
	if err != nil {
		return fmt.Errorf("unable to build Specs: %w", err)
	}

	for svcNameNs, pods := range res.PodsBySvcBySa[destSaKey] {
		svc := topology.Services[svcNameNs]
		svcKey := Key{svc.Name, svc.Namespace}

		svc, ok := topology.Services[svcKey]
		if !ok {
			return fmt.Errorf("unable to find Service %s/%s", svc.Namespace, svc.Name)
		}

		var destPods []*Pod

		// Find out which are the destination pods.
		for _, pod := range pods {
			if pod.Status.PodIP == "" {
				continue
			}

			destPods = append(destPods, getOrCreatePod(topology, pod))
		}

		// Find out which ports can be used on the destination service.
		destPorts, err := b.getTrafficTargetDestinationPorts(svc, tt)
		if err != nil {
			return fmt.Errorf("unable to find destination ports on Service %s/%s: %w", svc.Namespace, svc.Name, err)
		}

		// Create the ServiceTrafficTarget for the given service.
		svcTT := &ServiceTrafficTarget{
			Service: svc,
			Name:    tt.Name,
			Sources: sources,
			Destination: ServiceTrafficTargetDestination{
				ServiceAccount: tt.Destination.Name,
				Namespace:      tt.Destination.Namespace,
				Ports:          destPorts,
				Pods:           destPods,
			},
			Specs: specs,
		}
		svc.TrafficTargets = append(svc.TrafficTargets, svcTT)

		// Add the ServiceTrafficTarget to the source pods.
		for _, source := range sources {
			for _, pod := range source.Pods {
				pod.Outgoing = append(pod.Outgoing, svcTT)
			}
		}

		// Add the ServiceTrafficTarget to the destination pods.
		for _, pod := range svcTT.Destination.Pods {
			pod.Incoming = append(pod.Incoming, svcTT)
		}
	}

	return nil
}

// evaluateTrafficSplit evaluates the given traffic-split. If the traffic-split targets a known Service, a new TrafficSplit
// will be added to it. The TrafficSplit will be added only if all its backends expose the ports required by the Service.
func (b *Builder) evaluateTrafficSplit(topology *Topology, trafficSplit *split.TrafficSplit) error {
	svcKey := Key{trafficSplit.Spec.Service, trafficSplit.Namespace}

	svc, ok := topology.Services[svcKey]
	if !ok {
		return fmt.Errorf("unable to find root Service %s/%s", trafficSplit.Namespace, trafficSplit.Spec.Service)
	}

	ts := &TrafficSplit{
		Name:      trafficSplit.Name,
		Namespace: trafficSplit.Namespace,
		Service:   svc,
	}

	backends := make([]TrafficSplitBackend, len(trafficSplit.Spec.Backends))

	for i, backend := range trafficSplit.Spec.Backends {
		backendSvcKey := Key{backend.Service, trafficSplit.Namespace}

		backendSvc, ok := topology.Services[backendSvcKey]
		if !ok {
			return fmt.Errorf("unable to find backend Service %s/%s", trafficSplit.Namespace, backend.Service)
		}

		// As required by the SMI specification, backends must expose at least the same ports as the Service on
		// which the TrafficSplit is.
		for _, svcPort := range svc.Ports {
			var portFound bool

			for _, backendPort := range backendSvc.Ports {
				if svcPort.Port == backendPort.Port {
					portFound = true
					break
				}
			}

			if !portFound {
				return fmt.Errorf("port %d must be exposed by Service %s/%s in order to be used as a backend", svcPort.Port, backendSvc.Namespace, backendSvc.Name)
			}
		}

		backends[i] = TrafficSplitBackend{
			Weight:  backend.Weight,
			Service: backendSvc,
		}

		backendSvc.BackendOf = append(backendSvc.BackendOf, ts)
	}

	ts.Backends = backends
	svc.TrafficSplits = append(svc.TrafficSplits, ts)

	return nil
}

// populateTrafficSplitsAuthorizedIncomingTraffic computes the list of pods allowed to access a traffic-split. As
// traffic-splits may form a graph, it has to be done once all the traffic-splits have been processed. To avoid runtime
// issues, this method detects cycles in the graph.
func (b *Builder) populateTrafficSplitsAuthorizedIncomingTraffic(topology *Topology) {
	loopDetected := make(map[*Service][]*TrafficSplit)

	for _, svc := range topology.Services {
		for _, ts := range svc.TrafficSplits {
			pods, err := b.getIncomingPodsForTrafficSplit(ts, map[Key]struct{}{})
			if err != nil {
				loopDetected[svc] = append(loopDetected[svc], ts)
				b.Logger.Errorf("Unable to get incoming pods for TrafficSplit %s/%s: %v", ts.Namespace, ts.Name, err)

				continue
			}

			ts.Incoming = pods
		}
	}

	// Remove the TrafficSplits that causes a loop.
	for svc, tss := range loopDetected {
		for _, loopTs := range tss {
			for i, ts := range svc.TrafficSplits {
				if ts == loopTs {
					svc.TrafficSplits = append(svc.TrafficSplits[:i], svc.TrafficSplits[i+1:]...)
					break
				}
			}
		}
	}
}

func (b *Builder) getIncomingPodsForTrafficSplit(ts *TrafficSplit, visited map[Key]struct{}) ([]*Pod, error) {
	keyTS := Key{ts.Name, ts.Namespace}
	if _, found := visited[keyTS]; found {
		return nil, fmt.Errorf("circular reference detected on traffic split %s/%s in service %s/%s", ts.Namespace, ts.Name, ts.Service.Namespace, ts.Service.Name)
	}

	visited[keyTS] = struct{}{}

	var union []*Pod

	for _, backend := range ts.Backends {
		backendPods, err := b.getIncomingPodsForService(backend.Service, mapCopy(visited))
		if err != nil {
			return nil, err
		}

		union = unionPod(backendPods, union)

		if len(union) == 0 {
			return union, nil
		}
	}

	return union, nil
}

func (b *Builder) getIncomingPodsForService(svc *Service, visited map[Key]struct{}) ([]*Pod, error) {
	var union []*Pod

	if len(svc.TrafficSplits) == 0 {
		var pods []*Pod

		for _, tt := range svc.TrafficTargets {
			for _, source := range tt.Sources {
				pods = append(pods, source.Pods...)
			}
		}

		return pods, nil
	}

	for _, ts := range svc.TrafficSplits {
		tsPods, err := b.getIncomingPodsForTrafficSplit(ts, visited)
		if err != nil {
			return nil, err
		}

		union = unionPod(tsPods, union)

		if len(union) == 0 {
			return union, nil
		}
	}

	return union, nil
}

func unionPod(pods1, pods2 []*Pod) []*Pod {
	var union []*Pod

	if pods2 == nil {
		return pods1
	}

	p := map[*Pod]bool{}

	for _, pod := range pods1 {
		p[pod] = true
	}

	for _, pod := range pods2 {
		if _, found := p[pod]; found {
			union = append(union, pod)
		}
	}

	return union
}

// buildTrafficTargetSources retrieves the Pod IPs for each Pod mentioned in a source of the given TrafficTarget.
// If a Pod IP is not yet available, the pod will be skipped.
func (b *Builder) buildTrafficTargetSources(res *resources, t *Topology, tt *access.TrafficTarget) []ServiceTrafficTargetSource {
	sources := make([]ServiceTrafficTargetSource, len(tt.Sources))

	for i, source := range tt.Sources {
		srcSaKey := Key{source.Name, source.Namespace}

		pods := res.PodsBySa[srcSaKey]
		srcPods := make([]*Pod, len(pods))

		for k, pod := range pods {
			if pod.Status.PodIP == "" {
				continue
			}

			srcPods[k] = getOrCreatePod(t, pod)
		}

		sources[i] = ServiceTrafficTargetSource{
			ServiceAccount: source.Name,
			Namespace:      source.Namespace,
			Pods:           srcPods,
		}
	}

	return sources
}

func (b *Builder) buildTrafficTargetSpecs(res *resources, tt *access.TrafficTarget) ([]TrafficSpec, error) {
	var trafficSpecs []TrafficSpec

	for _, s := range tt.Specs {
		switch s.Kind {
		case "HTTPRouteGroup":
			trafficSpec, err := b.buildHTTPRouteGroup(res.HTTPRouteGroups, tt.Namespace, s)
			if err != nil {
				return []TrafficSpec{}, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		case "TCPRoute":
			trafficSpec, err := b.buildTCPRoute(res.TCPRoutes, tt.Namespace, s)
			if err != nil {
				return []TrafficSpec{}, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		default:
			return []TrafficSpec{}, fmt.Errorf("unknown spec type: %q", s.Kind)
		}
	}

	return trafficSpecs, nil
}

func (b *Builder) buildHTTPRouteGroup(httpRtGrps map[Key]*spec.HTTPRouteGroup, ns string, s access.TrafficTargetSpec) (TrafficSpec, error) {
	key := Key{s.Name, ns}

	httpRouteGroup, ok := httpRtGrps[key]
	if !ok {
		return TrafficSpec{}, fmt.Errorf("unable to find HTTPRouteGroup %s/%s", ns, s.Name)
	}

	var httpMatches []*spec.HTTPMatch

	if len(s.Matches) == 0 {
		httpMatches = make([]*spec.HTTPMatch, len(httpRouteGroup.Matches))

		for i, match := range httpRouteGroup.Matches {
			m := match
			httpMatches[i] = &m
		}
	} else {
		for _, name := range s.Matches {
			var found bool

			for _, match := range httpRouteGroup.Matches {
				found = match.Name == name

				if found {
					httpMatches = append(httpMatches, &match)
					break
				}
			}

			if !found {
				return TrafficSpec{}, fmt.Errorf("unable to find match %q in HTTPRouteGroup %s/%s", name, ns, s.Name)
			}
		}
	}

	return TrafficSpec{
		HTTPRouteGroup: httpRouteGroup,
		HTTPMatches:    httpMatches,
	}, nil
}

func (b *Builder) buildTCPRoute(tcpRts map[Key]*spec.TCPRoute, ns string, s access.TrafficTargetSpec) (TrafficSpec, error) {
	key := Key{s.Name, ns}

	tcpRoute, ok := tcpRts[key]
	if !ok {
		return TrafficSpec{}, fmt.Errorf("unable to find TCPRoute %s/%s", ns, s.Name)
	}

	return TrafficSpec{
		TCPRoute: tcpRoute,
	}, nil
}

// getTrafficTargetDestinationPorts gets the ports mentioned in the TrafficTarget.Destination.Port.
// If the port is "", all of the Service's ports are returned.
// If the port is an integer, it is returned.
func (b *Builder) getTrafficTargetDestinationPorts(svc *Service, tt *access.TrafficTarget) ([]v1.ServicePort, error) {
	if tt.Destination.Port == "" {
		return svc.Ports, nil
	}

	port, err := strconv.ParseInt(tt.Destination.Port, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("destination port of TrafficTarget %s/%s is not a valid port: %w", tt.Namespace, tt.Name, err)
	}

	for _, svcPort := range svc.Ports {
		if svcPort.TargetPort.IntVal == int32(port) {
			return []v1.ServicePort{svcPort}, nil
		}
	}

	return nil, fmt.Errorf("destination port %d of TrafficTarget %s/%s is not exposed by the service", port, tt.Namespace, tt.Name)
}

func getOrCreatePod(topology *Topology, pod *v1.Pod) *Pod {
	podKey := Key{pod.Name, pod.Namespace}

	if _, ok := topology.Pods[podKey]; !ok {
		topology.Pods[podKey] = &Pod{
			Name:           pod.Name,
			Namespace:      pod.Namespace,
			ServiceAccount: pod.Spec.ServiceAccountName,
			Owner:          pod.OwnerReferences,
			IP:             pod.Status.PodIP,
		}
	}

	return topology.Pods[podKey]
}

type resources struct {
	Services        map[Key]*v1.Service
	TrafficTargets  map[Key]*access.TrafficTarget
	TrafficSplits   map[Key]*split.TrafficSplit
	HTTPRouteGroups map[Key]*spec.HTTPRouteGroup
	TCPRoutes       map[Key]*spec.TCPRoute

	// Pods indexes.
	PodsBySvc     map[Key][]*v1.Pod
	PodsBySa      map[Key][]*v1.Pod
	PodsBySvcBySa map[Key]map[Key][]*v1.Pod
}

func (b *Builder) loadResources(ignoredResources mk8s.IgnoreWrapper) (*resources, error) {
	res := &resources{
		Services:        make(map[Key]*v1.Service),
		TrafficTargets:  make(map[Key]*access.TrafficTarget),
		TrafficSplits:   make(map[Key]*split.TrafficSplit),
		HTTPRouteGroups: make(map[Key]*spec.HTTPRouteGroup),
		TCPRoutes:       make(map[Key]*spec.TCPRoute),
		PodsBySvc:       make(map[Key][]*v1.Pod),
		PodsBySa:        make(map[Key][]*v1.Pod),
		PodsBySvcBySa:   make(map[Key]map[Key][]*v1.Pod),
	}

	svcs, err := b.ServiceLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Services: %w", err)
	}

	pods, err := b.PodLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Services: %w", err)
	}

	eps, err := b.EndpointsLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Services: %w", err)
	}

	tss, err := b.TrafficSplitLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list TrafficSplits: %w", err)
	}

	var httpRtGrps []*spec.HTTPRouteGroup
	if b.HTTPRouteGroupLister != nil {
		httpRtGrps, err = b.HTTPRouteGroupLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("unable to list HTTPRouteGroups: %w", err)
		}
	}

	var tcpRts []*spec.TCPRoute
	if b.TCPRoutesLister != nil {
		tcpRts, err = b.TCPRoutesLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("unable to list TCPRouteGroups: %w", err)
		}
	}

	var tts []*access.TrafficTarget
	if b.TrafficTargetLister != nil {
		tts, err = b.TrafficTargetLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("unable to list TrafficTargets: %w", err)
		}
	}

	for _, svc := range svcs {
		if ignoredResources.IsIgnored(svc.ObjectMeta) {
			continue
		}

		res.Services[Key{svc.Name, svc.Namespace}] = svc
	}

	b.indexSMIResources(res, ignoredResources, tts, tss, tcpRts, httpRtGrps)
	b.indexPods(res, ignoredResources, pods, eps)

	return res, nil
}

// indexPods populates the different pod indexes in the given resources object. It builds 3 indexes:
// - pods indexed by service-account
// - pods indexed by service
// - pods indexed by service indexed by service-account
func (b *Builder) indexPods(res *resources, ignoredResources mk8s.IgnoreWrapper, pods []*v1.Pod, eps []*v1.Endpoints) {
	podsByName := make(map[Key]*v1.Pod)

	for _, pod := range pods {
		if ignoredResources.IsIgnored(pod.ObjectMeta) {
			continue
		}

		keyPod := Key{Name: pod.Name, Namespace: pod.Namespace}
		podsByName[keyPod] = pod

		saKey := Key{pod.Spec.ServiceAccountName, pod.Namespace}
		res.PodsBySa[saKey] = append(res.PodsBySa[saKey], pod)
	}

	for _, ep := range eps {
		if ignoredResources.IsIgnored(ep.ObjectMeta) {
			continue
		}

		for _, subset := range ep.Subsets {
			for _, address := range subset.Addresses {
				if address.TargetRef == nil {
					continue
				}

				keyPod := Key{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}

				pod, ok := podsByName[keyPod]
				if !ok {
					continue
				}

				keySA := Key{Name: pod.Spec.ServiceAccountName, Namespace: pod.Namespace}
				keyEP := Key{Name: ep.Name, Namespace: ep.Namespace}

				if _, ok := res.PodsBySvcBySa[keySA]; !ok {
					res.PodsBySvcBySa[keySA] = make(map[Key][]*v1.Pod)
				}

				res.PodsBySvcBySa[keySA][keyEP] = append(res.PodsBySvcBySa[keySA][keyEP], pod)
				res.PodsBySvc[keyEP] = append(res.PodsBySvc[keyEP], pod)
			}
		}
	}
}

func (b *Builder) indexSMIResources(res *resources, ignoredResources mk8s.IgnoreWrapper, tts []*access.TrafficTarget, tss []*split.TrafficSplit, tcpRts []*spec.TCPRoute, httpRtGrps []*spec.HTTPRouteGroup) {
	for _, httpRouteGroup := range httpRtGrps {
		if ignoredResources.IsIgnored(httpRouteGroup.ObjectMeta) {
			continue
		}

		res.HTTPRouteGroups[Key{httpRouteGroup.Name, httpRouteGroup.Namespace}] = httpRouteGroup
	}

	for _, tcpRoute := range tcpRts {
		if ignoredResources.IsIgnored(tcpRoute.ObjectMeta) {
			continue
		}

		res.TCPRoutes[Key{tcpRoute.Name, tcpRoute.Namespace}] = tcpRoute
	}

	for _, trafficTarget := range tts {
		if ignoredResources.IsIgnored(trafficTarget.ObjectMeta) {
			continue
		}

		res.TrafficTargets[Key{trafficTarget.Name, trafficTarget.Namespace}] = trafficTarget
	}

	for _, trafficSplit := range tss {
		if ignoredResources.IsIgnored(trafficSplit.ObjectMeta) {
			continue
		}

		res.TrafficSplits[Key{trafficSplit.Name, trafficSplit.Namespace}] = trafficSplit
	}
}

func mapCopy(m map[Key]struct{}) map[Key]struct{} {
	copy := map[Key]struct{}{}

	for k, b := range m {
		copy[k] = b
	}

	return copy
}
