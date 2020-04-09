package topology

import (
	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NameNamespace is a key for referencing unique resources.
type NameNamespace struct {
	Name      string
	Namespace string
}

// Topology holds the graph. Each Pods and services are nodes of the graph.
type Topology struct {
	Services map[NameNamespace]*Service
	Pods     map[NameNamespace]*Pod
}

// NewTopology creates a new Topology.
func NewTopology() *Topology {
	return &Topology{
		Services: make(map[NameNamespace]*Service),
		Pods:     make(map[NameNamespace]*Pod),
	}
}

// Service is a node of the graph representing a kubernetes service.
type Service struct {
	Name        string
	Namespace   string
	Selector    map[string]string
	Annotations map[string]string
	Ports       []corev1.ServicePort
	ClusterIP   string
	Pods        []*Pod

	// List of TrafficTargets that are targeting pods which are selected by this service.
	TrafficTargets []*ServiceTrafficTarget
	// List of TrafficSplits that are targeting this service.
	TrafficSplits []*TrafficSplit
	// List of TrafficSplit mentioning this service as a backend.
	BackendOf []*TrafficSplit
}

// ServiceTrafficTarget represents a TrafficTarget applied a on Service. TrafficTargets have a Destination service
// account. This service account can be set on many pods, each of them, potentially accessible through different services.
// A ServiceTrafficTarget is a TrafficTarget for a Service which exposes a Pod which has the TrafficTarget Destination
// service-account.
type ServiceTrafficTarget struct {
	Service *Service
	Name    string

	Sources     []ServiceTrafficTargetSource
	Destination ServiceTrafficTargetDestination
	Specs       []TrafficSpec
}

// ServiceTrafficTargetSource represents a source of a ServiceTrafficTarget. In the SMI specification, a TrafficTarget
// has a list of sources, each of them being a service-account name. ServiceTrafficTargetSource represents this
// service-account, populated with the pods having this Service.
type ServiceTrafficTargetSource struct {
	ServiceAccount string
	Namespace      string
	Pods           []*Pod
}

// ServiceTrafficTargetDestination represents a destination of a ServiceTrafficTarget. In the SMI specification, a
// TrafficTarget has a destination service-account. ServiceTrafficTargetDestination holds the pods exposed by the
// Service which has this service-account.
type ServiceTrafficTargetDestination struct {
	ServiceAccount string
	Namespace      string
	Ports          []corev1.ServicePort
	Pods           []*Pod
}

// TrafficSpec represents a Spec which can be used for restricting access to a route in a TrafficTarget.
type TrafficSpec struct {
	HTTPRouteGroup *specs.HTTPRouteGroup
	TCPRoute       *specs.TCPRoute

	// HTTPMatches is the list of HTTPMatch selected from the HTTPRouteGroup.
	HTTPMatches []*specs.HTTPMatch
}

// Pod is a node of the graph representing a kubernetes pod.
type Pod struct {
	Name           string
	Namespace      string
	ServiceAccount string
	Owner          []v1.OwnerReference
	IP             string

	Outgoing []*ServiceTrafficTarget
	Incoming []*ServiceTrafficTarget
}

// TrafficSplit represents a TrafficSplit applied on a Service.
type TrafficSplit struct {
	Name      string
	Namespace string

	Service  *Service
	Backends []TrafficSplitBackend

	// List of Pods that are explicitly allowed to pass through the TrafficSplit.
	Incoming []*Pod
}

// TrafficSplitBackend is a backend of a TrafficSplit.
type TrafficSplitBackend struct {
	Weight  int
	Service *Service
}
