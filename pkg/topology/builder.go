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
		if err := b.evaluateService(res, topology, svc); err != nil {
			b.Logger.Errorf("Unable to evaluate Service %s/%s: %v", svc.Namespace, svc.Name, err)
		}
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

// evaluateService evaluates the given service. It adds the Service to the topology and it's selected Pods.
func (b *Builder) evaluateService(res *resources, topology *Topology, svc *v1.Service) error {
	svcKey := Key{svc.Name, svc.Namespace}

	svcPods, ok := res.PodsBySvc[svcKey]
	if !ok {
		return fmt.Errorf("unable to find Service %q", svcKey)
	}

	pods := make([]Key, len(svcPods))

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

	return nil
}

// evaluateTrafficTarget evaluates the given traffic-target. It adds a ServiceTrafficTargets on every Service which
// has pods with a service-account being the one defined in the traffic-target destination.
// When a ServiceTrafficTarget gets added to a Service, each source and destination pod will be added to the topology
// and linked to it.
func (b *Builder) evaluateTrafficTarget(res *resources, topology *Topology, tt *access.TrafficTarget) error {
	destSaKey := Key{tt.Destination.Name, tt.Destination.Namespace}

	sources, srcErr := b.buildTrafficTargetSources(res, topology, tt)
	if srcErr != nil {
		return fmt.Errorf("unable to build TrafficTarget sources: %w", srcErr)
	}

	specs, specsErr := b.buildTrafficTargetSpecs(res, tt)
	if specsErr != nil {
		return fmt.Errorf("unable to build Specs: %w", specsErr)
	}

	podsBySvc, ok := res.PodsBySvcBySa[destSaKey]
	if !ok {
		return fmt.Errorf("unable to find Pods with ServiceAccount %q", destSaKey)
	}

	for svcKey, pods := range podsBySvc {
		svc, ok := topology.Services[svcKey]
		if !ok {
			return fmt.Errorf("unable to find Service %q", svcKey)
		}

		var destPods []Key

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
			return fmt.Errorf("unable to find destination ports on Service %q: %w", svcKey, err)
		}

		// Create the ServiceTrafficTarget for the given service.
		svcTTKey := ServiceTrafficTargetKey{
			Service:       svcKey,
			TrafficTarget: Key{tt.Name, tt.Namespace},
		}
		topology.ServiceTrafficTargets[svcTTKey] = &ServiceTrafficTarget{
			Service:   svcKey,
			Name:      tt.Name,
			Namespace: tt.Namespace,
			Sources:   sources,
			Destination: ServiceTrafficTargetDestination{
				ServiceAccount: tt.Destination.Name,
				Namespace:      tt.Destination.Namespace,
				Ports:          destPorts,
				Pods:           destPods,
			},
			Specs: specs,
		}

		svc.TrafficTargets = append(svc.TrafficTargets, svcTTKey)

		// Add the ServiceTrafficTarget to the source pods.
		for _, source := range sources {
			for _, podKey := range source.Pods {
				// Skip pods which haven't been added to the topology.
				if _, ok := topology.Pods[podKey]; !ok {
					continue
				}

				topology.Pods[podKey].SourceOf = append(topology.Pods[podKey].SourceOf, svcTTKey)
			}
		}

		// Add the ServiceTrafficTarget to the destination pods.
		for _, podKey := range topology.ServiceTrafficTargets[svcTTKey].Destination.Pods {
			// Skip pods which haven't been added to the topology.
			if _, ok := topology.Pods[podKey]; !ok {
				continue
			}

			topology.Pods[podKey].DestinationOf = append(topology.Pods[podKey].DestinationOf, svcTTKey)
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
		return fmt.Errorf("unable to find root Service %q", svcKey)
	}

	tsKey := Key{trafficSplit.Name, trafficSplit.Namespace}
	backends := make([]TrafficSplitBackend, len(trafficSplit.Spec.Backends))

	for i, backend := range trafficSplit.Spec.Backends {
		backendSvcKey := Key{backend.Service, trafficSplit.Namespace}

		backendSvc, ok := topology.Services[backendSvcKey]
		if !ok {
			return fmt.Errorf("unable to find backend Service %q", backendSvcKey)
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
				return fmt.Errorf("port %d must be exposed by Service %q in order to be used as a backend", svcPort.Port, backendSvcKey)
			}
		}

		backends[i] = TrafficSplitBackend{
			Weight:  backend.Weight,
			Service: backendSvcKey,
		}

		backendSvc.BackendOf = append(backendSvc.BackendOf, tsKey)
	}

	topology.TrafficSplits[tsKey] = &TrafficSplit{
		Name:      trafficSplit.Name,
		Namespace: trafficSplit.Namespace,
		Service:   svcKey,
		Backends:  backends,
	}

	svc.TrafficSplits = append(svc.TrafficSplits, tsKey)

	return nil
}

// populateTrafficSplitsAuthorizedIncomingTraffic computes the list of pods allowed to access a traffic-split. As
// traffic-splits may form a graph, it has to be done once all the traffic-splits have been processed. To avoid runtime
// issues, this method detects cycles in the graph.
func (b *Builder) populateTrafficSplitsAuthorizedIncomingTraffic(topology *Topology) {
	loopDetected := make(map[*Service][]Key)

	for _, svc := range topology.Services {
		for _, tsKey := range svc.TrafficSplits {
			ts, ok := topology.TrafficSplits[tsKey]
			if !ok {
				b.Logger.Errorf("Unable to find TrafficSplit %q", tsKey)
				continue
			}

			pods, err := b.getIncomingPodsForTrafficSplit(topology, ts, map[Key]struct{}{})
			if err != nil {
				loopDetected[svc] = append(loopDetected[svc], tsKey)

				b.Logger.Errorf("Unable to get incoming pods for TrafficSplit %q: %v", tsKey, err)

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

func (b *Builder) getIncomingPodsForTrafficSplit(topology *Topology, ts *TrafficSplit, visited map[Key]struct{}) ([]Key, error) {
	tsKey := Key{ts.Name, ts.Namespace}
	if _, ok := visited[tsKey]; ok {
		return nil, fmt.Errorf("circular reference detected on TrafficSplit %q in Service %q", tsKey, ts.Service)
	}

	visited[tsKey] = struct{}{}

	var union []Key

	for _, backend := range ts.Backends {
		backendPods, err := b.getIncomingPodsForService(topology, backend.Service, mapCopy(visited))
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

func (b *Builder) getIncomingPodsForService(topology *Topology, svcKey Key, visited map[Key]struct{}) ([]Key, error) {
	var union []Key

	svc, ok := topology.Services[svcKey]
	if !ok {
		return nil, fmt.Errorf("unable to find Service %q", svcKey)
	}

	if len(svc.TrafficSplits) == 0 {
		var pods []Key

		for _, ttKey := range svc.TrafficTargets {
			tt, ok := topology.ServiceTrafficTargets[ttKey]
			if !ok {
				return nil, fmt.Errorf("unable to find TrafficTarget %q", ttKey)
			}

			for _, source := range tt.Sources {
				pods = append(pods, source.Pods...)
			}
		}

		return pods, nil
	}

	for _, tsKey := range svc.TrafficSplits {
		ts, ok := topology.TrafficSplits[tsKey]
		if !ok {
			return nil, fmt.Errorf("unable to find TrafficSplit %q", tsKey)
		}

		tsPods, err := b.getIncomingPodsForTrafficSplit(topology, ts, visited)
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

// unionPod returns the union of the given two slices.
func unionPod(pods1, pods2 []Key) []Key {
	var union []Key

	if pods2 == nil {
		return pods1
	}

	p := map[Key]struct{}{}

	for _, pod := range pods1 {
		key := Key{pod.Name, pod.Namespace}

		p[key] = struct{}{}
	}

	for _, pod := range pods2 {
		key := Key{pod.Name, pod.Namespace}

		if _, ok := p[key]; ok {
			union = append(union, pod)
		}
	}

	return union
}

// buildTrafficTargetSources retrieves the Pod IPs for each Pod mentioned in a source of the given TrafficTarget.
// If a Pod IP is not yet available, the pod will be skipped.
func (b *Builder) buildTrafficTargetSources(res *resources, t *Topology, tt *access.TrafficTarget) ([]ServiceTrafficTargetSource, error) {
	sources := make([]ServiceTrafficTargetSource, len(tt.Sources))

	for i, source := range tt.Sources {
		srcSaKey := Key{source.Name, source.Namespace}

		pods, ok := res.PodsByServiceAccounts[srcSaKey]
		if !ok {
			return nil, fmt.Errorf("unable to find Pods with ServiceAccount %q", srcSaKey)
		}

		srcPods := make([]Key, len(pods))

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

	return sources, nil
}

func (b *Builder) buildTrafficTargetSpecs(res *resources, tt *access.TrafficTarget) ([]TrafficSpec, error) {
	var trafficSpecs []TrafficSpec

	for _, s := range tt.Specs {
		switch s.Kind {
		case "HTTPRouteGroup":
			trafficSpec, err := b.buildHTTPRouteGroup(res.HTTPRouteGroups, tt.Namespace, s)
			if err != nil {
				return nil, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		case "TCPRoute":
			trafficSpec, err := b.buildTCPRoute(res.TCPRoutes, tt.Namespace, s)
			if err != nil {
				return nil, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		default:
			return nil, fmt.Errorf("unknown spec type: %q", s.Kind)
		}
	}

	return trafficSpecs, nil
}

func (b *Builder) buildHTTPRouteGroup(httpRtGrps map[Key]*spec.HTTPRouteGroup, ns string, s access.TrafficTargetSpec) (TrafficSpec, error) {
	key := Key{s.Name, ns}

	httpRouteGroup, ok := httpRtGrps[key]
	if !ok {
		return TrafficSpec{}, fmt.Errorf("unable to find HTTPRouteGroup %q", key)
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
				return TrafficSpec{}, fmt.Errorf("unable to find match %q in HTTPRouteGroup %q", name, key)
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
		return TrafficSpec{}, fmt.Errorf("unable to find TCPRoute %q", key)
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

	key := Key{tt.Name, tt.Namespace}

	port, err := strconv.ParseInt(tt.Destination.Port, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("destination port of TrafficTarget %q is not a valid port: %w", key, err)
	}

	for _, svcPort := range svc.Ports {
		if svcPort.TargetPort.IntVal == int32(port) {
			return []v1.ServicePort{svcPort}, nil
		}
	}

	return nil, fmt.Errorf("destination port %d of TrafficTarget %q is not exposed by the service", port, key)
}

func getOrCreatePod(topology *Topology, pod *v1.Pod) Key {
	podKey := Key{pod.Name, pod.Namespace}

	if _, ok := topology.Pods[podKey]; !ok {
		topology.Pods[podKey] = &Pod{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			ServiceAccount:  pod.Spec.ServiceAccountName,
			OwnerReferences: pod.OwnerReferences,
			IP:              pod.Status.PodIP,
		}
	}

	return podKey
}

func (b *Builder) loadResources(ignoredResources mk8s.IgnoreWrapper) (*resources, error) {
	res := &resources{
		Services:              make(map[Key]*v1.Service),
		TrafficTargets:        make(map[Key]*access.TrafficTarget),
		TrafficSplits:         make(map[Key]*split.TrafficSplit),
		HTTPRouteGroups:       make(map[Key]*spec.HTTPRouteGroup),
		TCPRoutes:             make(map[Key]*spec.TCPRoute),
		PodsBySvc:             make(map[Key][]*v1.Pod),
		PodsByServiceAccounts: make(map[Key][]*v1.Pod),
		PodsBySvcBySa:         make(map[Key]map[Key][]*v1.Pod),
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

	res.indexSMIResources(ignoredResources, tts, tss, tcpRts, httpRtGrps)
	res.indexPods(ignoredResources, pods, eps)

	return res, nil
}

type resources struct {
	Services        map[Key]*v1.Service
	TrafficTargets  map[Key]*access.TrafficTarget
	TrafficSplits   map[Key]*split.TrafficSplit
	HTTPRouteGroups map[Key]*spec.HTTPRouteGroup
	TCPRoutes       map[Key]*spec.TCPRoute

	// Pods indexes.
	PodsBySvc             map[Key][]*v1.Pod
	PodsByServiceAccounts map[Key][]*v1.Pod
	PodsBySvcBySa         map[Key]map[Key][]*v1.Pod
}

// indexPods populates the different pod indexes in the given resources object. It builds 3 indexes:
// - pods indexed by service-account
// - pods indexed by service
// - pods indexed by service indexed by service-account
func (r *resources) indexPods(ignoredResources mk8s.IgnoreWrapper, pods []*v1.Pod, eps []*v1.Endpoints) {
	podsByName := make(map[Key]*v1.Pod)

	for _, pod := range pods {
		if ignoredResources.IsIgnored(pod.ObjectMeta) {
			continue
		}

		keyPod := Key{Name: pod.Name, Namespace: pod.Namespace}
		podsByName[keyPod] = pod

		saKey := Key{pod.Spec.ServiceAccountName, pod.Namespace}
		r.PodsByServiceAccounts[saKey] = append(r.PodsByServiceAccounts[saKey], pod)
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

				if _, ok := r.PodsBySvcBySa[keySA]; !ok {
					r.PodsBySvcBySa[keySA] = make(map[Key][]*v1.Pod)
				}

				r.PodsBySvcBySa[keySA][keyEP] = append(r.PodsBySvcBySa[keySA][keyEP], pod)
				r.PodsBySvc[keyEP] = append(r.PodsBySvc[keyEP], pod)
			}
		}
	}
}

func (r *resources) indexSMIResources(ignoredResources mk8s.IgnoreWrapper, tts []*access.TrafficTarget, tss []*split.TrafficSplit, tcpRts []*spec.TCPRoute, httpRtGrps []*spec.HTTPRouteGroup) {
	for _, httpRouteGroup := range httpRtGrps {
		if ignoredResources.IsIgnored(httpRouteGroup.ObjectMeta) {
			continue
		}

		key := Key{httpRouteGroup.Name, httpRouteGroup.Namespace}
		r.HTTPRouteGroups[key] = httpRouteGroup
	}

	for _, tcpRoute := range tcpRts {
		if ignoredResources.IsIgnored(tcpRoute.ObjectMeta) {
			continue
		}

		key := Key{tcpRoute.Name, tcpRoute.Namespace}
		r.TCPRoutes[key] = tcpRoute
	}

	for _, trafficTarget := range tts {
		if ignoredResources.IsIgnored(trafficTarget.ObjectMeta) {
			continue
		}

		key := Key{trafficTarget.Name, trafficTarget.Namespace}
		r.TrafficTargets[key] = trafficTarget
	}

	for _, trafficSplit := range tss {
		if ignoredResources.IsIgnored(trafficSplit.ObjectMeta) {
			continue
		}

		key := Key{trafficSplit.Name, trafficSplit.Namespace}
		r.TrafficSplits[key] = trafficSplit
	}
}

func mapCopy(m map[Key]struct{}) map[Key]struct{} {
	cpy := map[Key]struct{}{}

	for k, b := range m {
		cpy[k] = b
	}

	return cpy
}
