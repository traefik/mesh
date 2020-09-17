package topology

import (
	"fmt"

	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha3"
	accesslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha2"
	speclister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha3"
	splitlister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha3"
	"github.com/sirupsen/logrus"
	mk8s "github.com/traefik/mesh/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"
)

// Builder builds Topology objects based on the current state of a kubernetes cluster.
type Builder struct {
	serviceLister        listers.ServiceLister
	endpointsLister      listers.EndpointsLister
	podLister            listers.PodLister
	trafficTargetLister  accesslister.TrafficTargetLister
	trafficSplitLister   splitlister.TrafficSplitLister
	httpRouteGroupLister speclister.HTTPRouteGroupLister
	tcpRoutesLister      speclister.TCPRouteLister
	logger               logrus.FieldLogger
}

// NewBuilder creates and returns a new topology Builder instance.
func NewBuilder(
	serviceLister listers.ServiceLister,
	endpointLister listers.EndpointsLister,
	podLister listers.PodLister,
	trafficTargetLister accesslister.TrafficTargetLister,
	trafficSplitLister splitlister.TrafficSplitLister,
	httpRouteGroupLister speclister.HTTPRouteGroupLister,
	tcpRoutesLister speclister.TCPRouteLister,
	logger logrus.FieldLogger,
) *Builder {
	return &Builder{
		serviceLister:        serviceLister,
		endpointsLister:      endpointLister,
		podLister:            podLister,
		trafficTargetLister:  trafficTargetLister,
		trafficSplitLister:   trafficSplitLister,
		httpRouteGroupLister: httpRouteGroupLister,
		tcpRoutesLister:      tcpRoutesLister,
		logger:               logger,
	}
}

// Build builds a graph representing the possible interactions between Pods and Services based on the current state
// of the kubernetes cluster.
func (b *Builder) Build(resourceFilter *mk8s.ResourceFilter) (*Topology, error) {
	topology := NewTopology()

	res, err := b.loadResources(resourceFilter)
	if err != nil {
		return nil, fmt.Errorf("unable to load resources: %w", err)
	}

	// Populate services.
	for _, svc := range res.Services {
		b.evaluateService(res, topology, svc)
	}

	// Populate services with traffic-split definitions.
	for _, ts := range res.TrafficSplits {
		b.evaluateTrafficSplit(res, topology, ts)
	}

	// Populate services with traffic-target definitions.
	for _, tt := range res.TrafficTargets {
		b.evaluateTrafficTarget(res, topology, tt)
	}

	b.populateTrafficSplitsAuthorizedIncomingTraffic(topology)

	return topology, nil
}

// evaluateService evaluates the given service. It adds the Service to the topology and its selected Pods.
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
	destSaKey := Key{tt.Spec.Destination.Name, tt.Spec.Destination.Namespace}

	sources := b.buildTrafficTargetSources(res, topology, tt)

	for svcKey, pods := range res.PodsBySvcBySa[destSaKey] {
		stt := &ServiceTrafficTarget{
			Name:      tt.Name,
			Namespace: tt.Namespace,
			Service:   svcKey,
			Sources:   sources,
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
			b.logger.Errorf("Error building topology for TrafficTarget %q: %v", svcTTKey.TrafficTarget, err)

			continue
		}

		var err error

		stt.Rules, err = b.buildTrafficTargetRules(res, tt)
		if err != nil {
			err = fmt.Errorf("unable to build spec: %v", err)
			stt.AddError(err)
			b.logger.Errorf("Error building topology for TrafficTarget %q: %v", Key{tt.Name, tt.Namespace}, err)

			continue
		}

		stt.Destination, err = b.buildTrafficTargetDestination(topology, tt, pods, svc)
		if err != nil {
			stt.AddError(err)
			b.logger.Errorf("Error building topology for TrafficTarget %q: %v", Key{tt.Name, tt.Namespace}, err)

			continue
		}

		svc.TrafficTargets = append(svc.TrafficTargets, svcTTKey)

		// Add the ServiceTrafficTarget to the source and destination pods.
		addSourceAndDestinationToPods(topology, sources, svcTTKey)
	}
}

func (b *Builder) buildTrafficTargetDestination(topology *Topology, tt *access.TrafficTarget, pods []*corev1.Pod, svc *Service) (ServiceTrafficTargetDestination, error) {
	dest := ServiceTrafficTargetDestination{
		ServiceAccount: tt.Spec.Destination.Name,
		Namespace:      tt.Spec.Destination.Namespace,
	}

	// Find out which are the destination pods.
	for _, pod := range pods {
		if pod.Status.PodIP == "" {
			continue
		}

		dest.Pods = append(dest.Pods, getOrCreatePod(topology, pod))
	}

	var err error

	// Find out which ports can be used on the destination service.
	dest.Ports, err = b.getTrafficTargetDestinationPorts(svc, tt)
	if err != nil {
		return dest, fmt.Errorf("unable to find destination ports on Service %q: %w", Key{Namespace: svc.Namespace, Name: svc.Name}, err)
	}

	return dest, nil
}

func addSourceAndDestinationToPods(topology *Topology, sources []ServiceTrafficTargetSource, svcTTKey ServiceTrafficTargetKey) {
	for _, source := range sources {
		for _, podKey := range source.Pods {
			// Skip pods which have not been added to the topology.
			if _, ok := topology.Pods[podKey]; !ok {
				continue
			}

			topology.Pods[podKey].SourceOf = append(topology.Pods[podKey].SourceOf, svcTTKey)
		}
	}

	for _, podKey := range topology.ServiceTrafficTargets[svcTTKey].Destination.Pods {
		// Skip pods which have not been added to the topology.
		if _, ok := topology.Pods[podKey]; !ok {
			continue
		}

		topology.Pods[podKey].DestinationOf = append(topology.Pods[podKey].DestinationOf, svcTTKey)
	}
}

// evaluateTrafficSplit evaluates the given traffic-split. If the traffic-split targets a known Service, a new TrafficSplit
// will be added to it. The TrafficSplit will be added only if all its backends expose the ports required by the Service.
func (b *Builder) evaluateTrafficSplit(res *resources, topology *Topology, trafficSplit *split.TrafficSplit) {
	svcKey := Key{trafficSplit.Spec.Service, trafficSplit.Namespace}
	ts := &TrafficSplit{
		Name:      trafficSplit.Name,
		Namespace: trafficSplit.Namespace,
		Service:   svcKey,
	}

	tsKey := Key{trafficSplit.Name, trafficSplit.Namespace}
	topology.TrafficSplits[tsKey] = ts

	var err error

	ts.Rules, err = b.buildTrafficSplitSpecs(res, trafficSplit)
	if err != nil {
		err = fmt.Errorf("unable to build spec: %v", err)
		ts.AddError(err)
		b.logger.Errorf("Error building topology for TrafficTarget %q: %v", tsKey, err)

		return
	}

	svc, ok := topology.Services[svcKey]
	if !ok {
		err := fmt.Errorf("unable to find root Service %q", svcKey)
		ts.AddError(err)
		b.logger.Errorf("Error building topology for TrafficSplit %q: %v", tsKey, err)

		return
	}

	for _, backend := range trafficSplit.Spec.Backends {
		backendSvcKey := Key{backend.Service, trafficSplit.Namespace}

		backendSvc, ok := topology.Services[backendSvcKey]
		if !ok {
			err := fmt.Errorf("unable to find backend Service %q", backendSvcKey)
			ts.AddError(err)
			b.logger.Errorf("Error building topology for TrafficSplit %q: %v", tsKey, err)

			continue
		}

		// As required by the SMI specification, backends must expose at least the same ports as the Service on
		// which the TrafficSplit is.
		if err := b.validateServiceAndBackendPorts(svc.Ports, backendSvc.Ports); err != nil {
			ts.AddError(err)
			b.logger.Errorf("Error building topology for TrafficSplit %q: backend %q and service %q ports mismatch: %v", tsKey, backendSvcKey, svcKey, err)

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
				b.logger.Errorf("Unable to find TrafficSplit %q", tsKey)
				continue
			}

			pods, err := b.getIncomingPodsForTrafficSplit(topology, ts, map[Key]struct{}{})
			if err != nil {
				loopCausingTrafficSplitsByService[svc] = append(loopCausingTrafficSplitsByService[svc], tsKey)

				err = fmt.Errorf("unable to get incoming pods: %v", err)
				ts.AddError(err)
				b.logger.Errorf("Error building topology for TrafficSplit %q: %v", tsKey, err)

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
	sources := make([]ServiceTrafficTargetSource, len(tt.Spec.Sources))

	for i, source := range tt.Spec.Sources {
		srcSaKey := Key{source.Name, source.Namespace}

		pods := res.PodsByServiceAccounts[srcSaKey]

		var srcPods []Key

		for _, pod := range pods {
			if pod.Status.PodIP == "" {
				continue
			}

			srcPods = append(srcPods, getOrCreatePod(t, pod))
		}

		sources[i] = ServiceTrafficTargetSource{
			ServiceAccount: source.Name,
			Namespace:      source.Namespace,
			Pods:           srcPods,
		}
	}

	return sources
}

func (b *Builder) buildTrafficTargetRules(res *resources, tt *access.TrafficTarget) ([]TrafficSpec, error) {
	var trafficSpecs []TrafficSpec

	for _, s := range tt.Spec.Rules {
		switch s.Kind {
		case mk8s.HTTPRouteGroupObjectKind:
			trafficSpec, err := b.buildHTTPRouteGroup(res.HTTPRouteGroups, tt.Namespace, s.Name, s.Matches)
			if err != nil {
				return nil, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		case mk8s.TCPRouteObjectKind:
			trafficSpec, err := b.buildTCPRoute(res.TCPRoutes, tt.Namespace, s.Name)
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

func (b *Builder) buildTrafficSplitSpecs(res *resources, ts *split.TrafficSplit) ([]TrafficSpec, error) {
	var trafficSpecs []TrafficSpec

	for _, m := range ts.Spec.Matches {
		switch m.Kind {
		case mk8s.HTTPRouteGroupObjectKind:
			trafficSpec, err := b.buildHTTPRouteGroup(res.HTTPRouteGroups, ts.Namespace, m.Name, nil)
			if err != nil {
				return nil, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		case mk8s.TCPRouteObjectKind:
			trafficSpec, err := b.buildTCPRoute(res.TCPRoutes, ts.Namespace, m.Name)
			if err != nil {
				return nil, err
			}

			trafficSpecs = append(trafficSpecs, trafficSpec)
		default:
			return nil, fmt.Errorf("unknown spec type: %q", m.Kind)
		}
	}

	return trafficSpecs, nil
}

func (b *Builder) buildHTTPRouteGroup(httpRtGrps map[Key]*specs.HTTPRouteGroup, ns, name string, matches []string) (TrafficSpec, error) {
	key := Key{name, ns}

	httpRouteGroup, ok := httpRtGrps[key]
	if !ok {
		return TrafficSpec{}, fmt.Errorf("unable to find HTTPRouteGroup %q", key)
	}

	var (
		httpMatches []*specs.HTTPMatch
		err         error
	)

	if len(matches) == 0 {
		httpMatches = make([]*specs.HTTPMatch, len(httpRouteGroup.Spec.Matches))

		for i, match := range httpRouteGroup.Spec.Matches {
			m := match
			httpMatches[i] = &m
		}
	} else {
		httpMatches, err = buildHTTPRouteGroupMatches(matches, httpRouteGroup.Spec.Matches, httpMatches, key)
		if err != nil {
			return TrafficSpec{}, err
		}
	}

	return TrafficSpec{
		HTTPRouteGroup: httpRouteGroup,
		HTTPMatches:    httpMatches,
	}, nil
}

func buildHTTPRouteGroupMatches(ttMatches []string, httpRouteGroupMatches []specs.HTTPMatch, httpMatches []*specs.HTTPMatch, key Key) ([]*specs.HTTPMatch, error) {
	for _, name := range ttMatches {
		var found bool

		for _, match := range httpRouteGroupMatches {
			found = match.Name == name

			if found {
				match := match
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

func (b *Builder) buildTCPRoute(tcpRts map[Key]*specs.TCPRoute, ns, name string) (TrafficSpec, error) {
	key := Key{name, ns}

	tcpRoute, ok := tcpRts[key]
	if !ok {
		return TrafficSpec{}, fmt.Errorf("unable to find TCPRoute %q", key)
	}

	return TrafficSpec{
		TCPRoute: tcpRoute,
	}, nil
}

// getTrafficTargetDestinationPorts gets the ports mentioned in the TrafficTarget.Destination.Port. If the destination
// port is defined but not on the service itself an error will be returned. If the destination port is not defined, the
// traffic allowed on all the service's ports.
func (b *Builder) getTrafficTargetDestinationPorts(svc *Service, tt *access.TrafficTarget) ([]corev1.ServicePort, error) {
	port := tt.Spec.Destination.Port

	if port == nil {
		return svc.Ports, nil
	}

	key := Key{tt.Name, tt.Namespace}

	for _, svcPort := range svc.Ports {
		if svcPort.TargetPort.IntVal == int32(*port) {
			return []corev1.ServicePort{svcPort}, nil
		}
	}

	return nil, fmt.Errorf("destination port %d of TrafficTarget %q is not exposed by the service", *port, key)
}

func getOrCreatePod(topology *Topology, pod *corev1.Pod) Key {
	podKey := Key{pod.Name, pod.Namespace}

	if _, ok := topology.Pods[podKey]; !ok {
		var containerPorts []corev1.ContainerPort

		for _, container := range pod.Spec.Containers {
			containerPorts = append(containerPorts, container.Ports...)
		}

		topology.Pods[podKey] = &Pod{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			ServiceAccount:  pod.Spec.ServiceAccountName,
			OwnerReferences: pod.OwnerReferences,
			ContainerPorts:  containerPorts,
			IP:              pod.Status.PodIP,
		}
	}

	return podKey
}

func (b *Builder) loadResources(resourceFilter *mk8s.ResourceFilter) (*resources, error) {
	res := &resources{
		Services:              make(map[Key]*corev1.Service),
		TrafficTargets:        make(map[Key]*access.TrafficTarget),
		TrafficSplits:         make(map[Key]*split.TrafficSplit),
		HTTPRouteGroups:       make(map[Key]*specs.HTTPRouteGroup),
		TCPRoutes:             make(map[Key]*specs.TCPRoute),
		PodsBySvc:             make(map[Key][]*corev1.Pod),
		PodsByServiceAccounts: make(map[Key][]*corev1.Pod),
		PodsBySvcBySa:         make(map[Key]map[Key][]*corev1.Pod),
	}

	err := b.loadServices(resourceFilter, res)
	if err != nil {
		return nil, fmt.Errorf("unable to load Services: %w", err)
	}

	pods, err := b.podLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Pods: %w", err)
	}

	eps, err := b.endpointsLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list Endpoints: %w", err)
	}

	tss, err := b.trafficSplitLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list TrafficSplits: %w", err)
	}

	var httpRtGrps []*specs.HTTPRouteGroup
	if b.httpRouteGroupLister != nil {
		httpRtGrps, err = b.httpRouteGroupLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("unable to list HTTPRouteGroups: %w", err)
		}
	}

	var tcpRts []*specs.TCPRoute
	if b.tcpRoutesLister != nil {
		tcpRts, err = b.tcpRoutesLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("unable to list TCPRouteGroups: %w", err)
		}
	}

	var tts []*access.TrafficTarget
	if b.trafficTargetLister != nil {
		tts, err = b.trafficTargetLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("unable to list TrafficTargets: %w", err)
		}
	}

	res.indexSMIResources(resourceFilter, tts, tss, tcpRts, httpRtGrps)
	res.indexPods(resourceFilter, pods, eps)

	return res, nil
}

func (b *Builder) loadServices(resourceFilter *mk8s.ResourceFilter, res *resources) error {
	svcs, err := b.serviceLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("unable to list Services: %w", err)
	}

	for _, svc := range svcs {
		if resourceFilter.IsIgnored(svc) {
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
	HTTPRouteGroups map[Key]*specs.HTTPRouteGroup
	TCPRoutes       map[Key]*specs.TCPRoute

	// Pods indexes.
	PodsBySvc             map[Key][]*corev1.Pod
	PodsByServiceAccounts map[Key][]*corev1.Pod
	PodsBySvcBySa         map[Key]map[Key][]*corev1.Pod
}

// indexPods populates the different pod indexes in the given resources object. It builds 3 indexes:
// - pods indexed by service-account
// - pods indexed by service
// - pods indexed by service indexed by service-account.
func (r *resources) indexPods(resourceFilter *mk8s.ResourceFilter, pods []*corev1.Pod, eps []*corev1.Endpoints) {
	podsByName := make(map[Key]*corev1.Pod)

	r.indexPodsByServiceAccount(resourceFilter, pods, podsByName)
	r.indexPodsByService(resourceFilter, eps, podsByName)
}

func (r *resources) indexPodsByServiceAccount(resourceFilter *mk8s.ResourceFilter, pods []*corev1.Pod, podsByName map[Key]*corev1.Pod) {
	for _, pod := range pods {
		if resourceFilter.IsIgnored(pod) {
			continue
		}

		keyPod := Key{Name: pod.Name, Namespace: pod.Namespace}
		podsByName[keyPod] = pod

		saKey := Key{pod.Spec.ServiceAccountName, pod.Namespace}
		r.PodsByServiceAccounts[saKey] = append(r.PodsByServiceAccounts[saKey], pod)
	}
}

func (r *resources) indexPodsByService(resourceFilter *mk8s.ResourceFilter, eps []*corev1.Endpoints, podsByName map[Key]*corev1.Pod) {
	for _, ep := range eps {
		if resourceFilter.IsIgnored(ep) {
			continue
		}

		// This map keeps track of service pods already indexed. A service pod can be listed in multiple endpoint
		// subset in function of the matched service ports.
		indexedServicePods := make(map[Key]struct{})

		for _, subset := range ep.Subsets {
			for _, address := range subset.Addresses {
				r.indexPodByService(ep, address, podsByName, indexedServicePods)
			}
		}
	}
}

func (r *resources) indexPodByService(ep *corev1.Endpoints, address corev1.EndpointAddress, podsByName map[Key]*corev1.Pod, indexedServicePods map[Key]struct{}) {
	if address.TargetRef == nil {
		return
	}

	keyPod := Key{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}

	if _, exists := indexedServicePods[keyPod]; exists {
		return
	}

	pod, ok := podsByName[keyPod]
	if !ok {
		return
	}

	keySA := Key{Name: pod.Spec.ServiceAccountName, Namespace: pod.Namespace}
	keyEP := Key{Name: ep.Name, Namespace: ep.Namespace}

	if _, exists := r.PodsBySvcBySa[keySA]; !exists {
		r.PodsBySvcBySa[keySA] = make(map[Key][]*corev1.Pod)
	}

	r.PodsBySvcBySa[keySA][keyEP] = append(r.PodsBySvcBySa[keySA][keyEP], pod)
	r.PodsBySvc[keyEP] = append(r.PodsBySvc[keyEP], pod)

	indexedServicePods[keyPod] = struct{}{}
}

func (r *resources) indexSMIResources(resourceFilter *mk8s.ResourceFilter, tts []*access.TrafficTarget, tss []*split.TrafficSplit, tcpRts []*specs.TCPRoute, httpRtGrps []*specs.HTTPRouteGroup) {
	for _, httpRouteGroup := range httpRtGrps {
		if resourceFilter.IsIgnored(httpRouteGroup) {
			continue
		}

		key := Key{httpRouteGroup.Name, httpRouteGroup.Namespace}
		r.HTTPRouteGroups[key] = httpRouteGroup
	}

	for _, tcpRoute := range tcpRts {
		if resourceFilter.IsIgnored(tcpRoute) {
			continue
		}

		key := Key{tcpRoute.Name, tcpRoute.Namespace}
		r.TCPRoutes[key] = tcpRoute
	}

	for _, trafficTarget := range tts {
		if resourceFilter.IsIgnored(trafficTarget) {
			continue
		}

		// If the destination namespace is empty or blank, set it to the trafficTarget namespace.
		if trafficTarget.Spec.Destination.Namespace == "" {
			trafficTarget.Spec.Destination.Namespace = trafficTarget.Namespace
		}

		key := Key{trafficTarget.Name, trafficTarget.Namespace}
		r.TrafficTargets[key] = trafficTarget
	}

	for _, trafficSplit := range tss {
		if resourceFilter.IsIgnored(trafficSplit) {
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
