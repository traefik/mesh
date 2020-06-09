package topology

import (
	"fmt"
	"strconv"

	mk8s "github.com/containous/maesh/pkg/k8s"
	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha1"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha1"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	accesslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	speclister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitlister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"
)

// Builder builds Topology objects based on the current state of a kubernetes cluster.
type Builder struct {
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	PodLister            listers.PodLister
	TrafficTargetLister  accesslister.TrafficTargetLister
	TrafficSplitLister   splitlister.TrafficSplitLister
	HTTPRouteGroupLister speclister.HTTPRouteGroupLister
	TCPRoutesLister      speclister.TCPRouteLister
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
		b.evaluateTrafficTarget(res, topology, tt)
	}

	// Populate services with traffic-split definitions.
	for _, ts := range res.TrafficSplits {
		b.evaluateTrafficSplit(topology, ts)
	}

	b.populateTrafficSplitsAuthorizedIncomingTraffic(topology)

	return topology, nil
}

// evaluateService evaluates the given service. It adds the Service to the topology and it's selected Pods.
func (b *Builder) evaluateService(res *resources, topology *Topology, svc *corev1.Service) {
	svcKey := Key{svc.Name, svc.Namespace}

	svcPods := res.PodsBySvc[svcKey]
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
}

// evaluateTrafficTarget evaluates the given traffic-target. It adds a ServiceTrafficTargets on every Service which
// has pods with a service-account being the one defined in the traffic-target destination.
// When a ServiceTrafficTarget gets added to a Service, each source and destination pod will be added to the topology
// and linked to it.
func (b *Builder) evaluateTrafficTarget(res *resources, topology *Topology, tt *access.TrafficTarget) {
	destSaKey := Key{tt.Destination.Name, tt.Destination.Namespace}

	sources := b.buildTrafficTargetSources(res, topology, tt)

	for svcKey, pods := range res.PodsBySvcBySa[destSaKey] {
		stt := &ServiceTrafficTarget{
			Name:      tt.Name,
			Namespace: tt.Namespace,
			Service:   svcKey,
			Sources:   sources,
			Destination: ServiceTrafficTargetDestination{
				ServiceAccount: tt.Destination.Name,
				Namespace:      tt.Destination.Namespace,
			},
		}

		svcTTKey := ServiceTrafficTargetKey{
			Service:       svcKey,
			TrafficTarget: Key{tt.Name, tt.Namespace},
		}
		topology.ServiceTrafficTargets[svcTTKey] = stt

		svc, ok := topology.Services[svcKey]
		if !ok {
			err := fmt.Errorf("unable to find Service %q", svcKey)
			stt.AddError(err)
			b.Logger.Errorf("Error building topology for TrafficTarget %q: %v", Key{tt.Name, tt.Namespace}, err)

			continue
		}

		// Build the TrafficTarget Specs.
		specs, err := b.buildTrafficTargetSpecs(res, tt)
		if err != nil {
			err = fmt.Errorf("unable to build spec: %v", err)
			stt.AddError(err)
			b.Logger.Errorf("Error building topology for TrafficTarget %q: %v", Key{tt.Name, tt.Namespace}, err)

			continue
		}

		stt.Specs = specs

		// Find out which are the destination pods.
		for _, pod := range pods {
			if pod.Status.PodIP == "" {
				continue
			}

			stt.Destination.Pods = append(stt.Destination.Pods, getOrCreatePod(topology, pod))
		}

		// Find out which ports can be used on the destination service.
		destPorts, err := b.getTrafficTargetDestinationPorts(svc, tt)
		if err != nil {
			err = fmt.Errorf("unable to find destination ports on Service %q: %w", svcKey, err)
			stt.AddError(err)
			b.Logger.Errorf("Error building topology for TrafficTarget %q: %v", Key{tt.Name, tt.Namespace}, err)

			continue
		}

		stt.Destination.Ports = destPorts

		svc.TrafficTargets = append(svc.TrafficTargets, svcTTKey)

		// Add the ServiceTrafficTarget to the source and destination pods.
		addSourceAndDestinationToPods(topology, sources, svcTTKey)
	}
}

func addSourceAndDestinationToPods(topology *Topology, sources []ServiceTrafficTargetSource, svcTTKey ServiceTrafficTargetKey) {
	for _, source := range sources {
		for _, podKey := range source.Pods {
			// Skip pods which haven't been added to the topology.
			if _, ok := topology.Pods[podKey]; !ok {
				continue
			}

			topology.Pods[podKey].SourceOf = append(topology.Pods[podKey].SourceOf, svcTTKey)
		}
	}

	for _, podKey := range topology.ServiceTrafficTargets[svcTTKey].Destination.Pods {
		// Skip pods which haven't been added to the topology.
		if _, ok := topology.Pods[podKey]; !ok {
			continue
		}

		topology.Pods[podKey].DestinationOf = append(topology.Pods[podKey].DestinationOf, svcTTKey)
	}
}

// evaluateTrafficSplit evaluates the given traffic-split. If the traffic-split targets a known Service, a new TrafficSplit
// will be added to it. The TrafficSplit will be added only if all its backends expose the ports required by the Service.
func (b *Builder) evaluateTrafficSplit(topology *Topology, trafficSplit *split.TrafficSplit) {
	svcKey := Key{trafficSplit.Spec.Service, trafficSplit.Namespace}
	ts := &TrafficSplit{
		Name:      trafficSplit.Name,
		Namespace: trafficSplit.Namespace,
		Service:   svcKey,
	}

	tsKey := Key{trafficSplit.Name, trafficSplit.Namespace}
	topology.TrafficSplits[tsKey] = ts

	svc, ok := topology.Services[svcKey]
	if !ok {
		err := fmt.Errorf("unable to find root Service %q", svcKey)
		ts.AddError(err)
		b.Logger.Errorf("Error building topology for TrafficSplit %q: %v", tsKey, err)

		return
	}

	for _, backend := range trafficSplit.Spec.Backends {
		backendSvcKey := Key{backend.Service, trafficSplit.Namespace}

		backendSvc, ok := topology.Services[backendSvcKey]
		if !ok {
			err := fmt.Errorf("unable to find backend Service %q", backendSvcKey)
			ts.AddError(err)
			b.Logger.Errorf("Error building topology for TrafficSplit %q: %v", tsKey, err)

			continue
		}

		// As required by the SMI specification, backends must expose at least the same ports as the Service on
		// which the TrafficSplit is.
		if err := b.validateServiceAndBackendPorts(svc.Ports, backendSvc.Ports); err != nil {
			ts.AddError(err)
			b.Logger.Errorf("Error building topology for TrafficSplit %q: backend %q and service %q ports mismatch: %v", tsKey, backendSvcKey, svcKey, err)

			continue
		}

		ts.Backends = append(ts.Backends, TrafficSplitBackend{
			Weight:  backend.Weight,
			Service: backendSvcKey,
		})

		backendSvc.BackendOf = append(backendSvc.BackendOf, tsKey)
	}

	svc.TrafficSplits = append(svc.TrafficSplits, tsKey)
}

func (b *Builder) validateServiceAndBackendPorts(svcPorts []corev1.ServicePort, backendPorts []corev1.ServicePort) error {
	for _, svcPort := range svcPorts {
		var portFound bool

		for _, backendPort := range backendPorts {
			if svcPort.Port == backendPort.Port {
				portFound = true
				break
			}
		}

		if !portFound {
			return fmt.Errorf("port %d must be exposed", svcPort.Port)
		}
	}

	return nil
}

// populateTrafficSplitsAuthorizedIncomingTraffic computes the list of pods allowed to access a traffic-split. As
// traffic-splits may form a graph, it has to be done once all the traffic-splits have been processed. To avoid runtime
// issues, this method detects cycles in the graph.
func (b *Builder) populateTrafficSplitsAuthorizedIncomingTraffic(topology *Topology) {
	loopCausingTrafficSplitsByService := make(map[*Service][]Key)

	for _, svc := range topology.Services {
		for _, tsKey := range svc.TrafficSplits {
			ts, ok := topology.TrafficSplits[tsKey]
			if !ok {
				b.Logger.Errorf("Unable to find TrafficSplit %q", tsKey)
				continue
			}

			pods, err := b.getIncomingPodsForTrafficSplit(topology, ts, map[Key]struct{}{})
			if err != nil {
				loopCausingTrafficSplitsByService[svc] = append(loopCausingTrafficSplitsByService[svc], tsKey)

				err = fmt.Errorf("unable to get incoming pods: %v", err)
				ts.AddError(err)
				b.Logger.Errorf("Error building topology for TrafficSplit %q: %v", tsKey, err)

				continue
			}

			ts.Incoming = pods
		}
	}

	// Remove the TrafficSplits that were detected to cause a loop.
	removeLoopCausingTrafficSplits(loopCausingTrafficSplitsByService)
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
		return getPodsForServiceWithNoTrafficSplits(topology, svc)
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

func getPodsForServiceWithNoTrafficSplits(topology *Topology, svc *Service) ([]Key, error) {
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
func (b *Builder) buildTrafficTargetSources(res *resources, t *Topology, tt *access.TrafficTarget) []ServiceTrafficTargetSource {
	sources := make([]ServiceTrafficTargetSource, len(tt.Sources))

	for i, source := range tt.Sources {
		srcSaKey := Key{source.Name, source.Namespace}

		pods := res.PodsByServiceAccounts[srcSaKey]
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

	return sources
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

	var (
		httpMatches []*spec.HTTPMatch
		err         error
	)

	if len(s.Matches) == 0 {
		httpMatches = make([]*spec.HTTPMatch, len(httpRouteGroup.Matches))

		for i, match := range httpRouteGroup.Matches {
			m := match
			httpMatches[i] = &m
		}
	} else {
		httpMatches, err = buildHTTPRouteGroupMatches(s.Matches, httpRouteGroup.Matches, httpMatches, key)
		if err != nil {
			return TrafficSpec{}, err
		}
	}

	return TrafficSpec{
		HTTPRouteGroup: httpRouteGroup,
		HTTPMatches:    httpMatches,
	}, nil
}

func buildHTTPRouteGroupMatches(ttMatches []string, httpRouteGroupMatches []spec.HTTPMatch, httpMatches []*spec.HTTPMatch, key Key) ([]*spec.HTTPMatch, error) {
	for _, name := range ttMatches {
		var found bool

		for _, match := range httpRouteGroupMatches {
			found = match.Name == name

			if found {
				httpMatches = append(httpMatches, &match)
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("unable to find match %q in HTTPRouteGroup %q", name, key)
		}
	}

	return httpMatches, nil
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
func (b *Builder) getTrafficTargetDestinationPorts(svc *Service, tt *access.TrafficTarget) ([]corev1.ServicePort, error) {
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
			return []corev1.ServicePort{svcPort}, nil
		}
	}

	return nil, fmt.Errorf("destination port %d of TrafficTarget %q is not exposed by the service", port, key)
}

func getOrCreatePod(topology *Topology, pod *corev1.Pod) Key {
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
		Services:              make(map[Key]*corev1.Service),
		TrafficTargets:        make(map[Key]*access.TrafficTarget),
		TrafficSplits:         make(map[Key]*split.TrafficSplit),
		HTTPRouteGroups:       make(map[Key]*spec.HTTPRouteGroup),
		TCPRoutes:             make(map[Key]*spec.TCPRoute),
		PodsBySvc:             make(map[Key][]*corev1.Pod),
		PodsByServiceAccounts: make(map[Key][]*corev1.Pod),
		PodsBySvcBySa:         make(map[Key]map[Key][]*corev1.Pod),
	}

	err := b.loadServices(ignoredResources, res)
	if err != nil {
		return nil, fmt.Errorf("unable to load Services: %w", err)
	}

	pods, err := b.PodLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Pods: %w", err)
	}

	eps, err := b.EndpointsLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Endpoints: %w", err)
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

	res.indexSMIResources(ignoredResources, tts, tss, tcpRts, httpRtGrps)
	res.indexPods(ignoredResources, pods, eps)

	return res, nil
}

func (b *Builder) loadServices(ignoredResources mk8s.IgnoreWrapper, res *resources) error {
	svcs, err := b.ServiceLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("unable to list Services: %w", err)
	}

	for _, svc := range svcs {
		if ignoredResources.IsIgnored(svc) {
			continue
		}

		res.Services[Key{svc.Name, svc.Namespace}] = svc
	}

	return nil
}

type resources struct {
	Services        map[Key]*corev1.Service
	TrafficTargets  map[Key]*access.TrafficTarget
	TrafficSplits   map[Key]*split.TrafficSplit
	HTTPRouteGroups map[Key]*spec.HTTPRouteGroup
	TCPRoutes       map[Key]*spec.TCPRoute

	// Pods indexes.
	PodsBySvc             map[Key][]*corev1.Pod
	PodsByServiceAccounts map[Key][]*corev1.Pod
	PodsBySvcBySa         map[Key]map[Key][]*corev1.Pod
}

// indexPods populates the different pod indexes in the given resources object. It builds 3 indexes:
// - pods indexed by service-account
// - pods indexed by service
// - pods indexed by service indexed by service-account.
func (r *resources) indexPods(ignoredResources mk8s.IgnoreWrapper, pods []*corev1.Pod, eps []*corev1.Endpoints) {
	podsByName := make(map[Key]*corev1.Pod)

	r.indexPodsByServiceAccount(ignoredResources, pods, podsByName)
	r.indexPodsByService(ignoredResources, eps, podsByName)
}

func (r *resources) indexPodsByServiceAccount(ignoredResources mk8s.IgnoreWrapper, pods []*corev1.Pod, podsByName map[Key]*corev1.Pod) {
	for _, pod := range pods {
		if ignoredResources.IsIgnored(pod) {
			continue
		}

		keyPod := Key{Name: pod.Name, Namespace: pod.Namespace}
		podsByName[keyPod] = pod

		saKey := Key{pod.Spec.ServiceAccountName, pod.Namespace}
		r.PodsByServiceAccounts[saKey] = append(r.PodsByServiceAccounts[saKey], pod)
	}
}

func (r *resources) indexPodsByService(ignoredResources mk8s.IgnoreWrapper, eps []*corev1.Endpoints, podsByName map[Key]*corev1.Pod) {
	for _, ep := range eps {
		if ignoredResources.IsIgnored(ep) {
			continue
		}

		for _, subset := range ep.Subsets {
			for _, address := range subset.Addresses {
				r.indexPodByService(ep, address, podsByName)
			}
		}
	}
}

func (r *resources) indexPodByService(ep *corev1.Endpoints, address corev1.EndpointAddress, podsByName map[Key]*corev1.Pod) {
	if address.TargetRef == nil {
		return
	}

	keyPod := Key{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}

	pod, ok := podsByName[keyPod]
	if !ok {
		return
	}

	keySA := Key{Name: pod.Spec.ServiceAccountName, Namespace: pod.Namespace}
	keyEP := Key{Name: ep.Name, Namespace: ep.Namespace}

	if _, ok := r.PodsBySvcBySa[keySA]; !ok {
		r.PodsBySvcBySa[keySA] = make(map[Key][]*corev1.Pod)
	}

	r.PodsBySvcBySa[keySA][keyEP] = append(r.PodsBySvcBySa[keySA][keyEP], pod)
	r.PodsBySvc[keyEP] = append(r.PodsBySvc[keyEP], pod)
}

func (r *resources) indexSMIResources(ignoredResources mk8s.IgnoreWrapper, tts []*access.TrafficTarget, tss []*split.TrafficSplit, tcpRts []*spec.TCPRoute, httpRtGrps []*spec.HTTPRouteGroup) {
	for _, httpRouteGroup := range httpRtGrps {
		if ignoredResources.IsIgnored(httpRouteGroup) {
			continue
		}

		key := Key{httpRouteGroup.Name, httpRouteGroup.Namespace}
		r.HTTPRouteGroups[key] = httpRouteGroup
	}

	for _, tcpRoute := range tcpRts {
		if ignoredResources.IsIgnored(tcpRoute) {
			continue
		}

		key := Key{tcpRoute.Name, tcpRoute.Namespace}
		r.TCPRoutes[key] = tcpRoute
	}

	for _, trafficTarget := range tts {
		if ignoredResources.IsIgnored(trafficTarget) {
			continue
		}

		// If the destination namepace is empty or blank, set it to the trafficTarget namespace.
		if trafficTarget.Destination.Namespace == "" {
			trafficTarget.Destination.Namespace = trafficTarget.Namespace
		}

		key := Key{trafficTarget.Name, trafficTarget.Namespace}
		r.TrafficTargets[key] = trafficTarget
	}

	for _, trafficSplit := range tss {
		if ignoredResources.IsIgnored(trafficSplit) {
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

func removeLoopCausingTrafficSplits(loopCausingTrafficSplitsByService map[*Service][]Key) {
	for svc, tss := range loopCausingTrafficSplitsByService {
		for _, loopTS := range tss {
			for i, ts := range svc.TrafficSplits {
				if ts == loopTS {
					svc.TrafficSplits = append(svc.TrafficSplits[:i], svc.TrafficSplits[i+1:]...)
					break
				}
			}
		}
	}
}
