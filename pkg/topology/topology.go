package topology

import (
	"fmt"
	"strings"

	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Key references a resource.
type Key struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// String stringifies the Key.
func (k Key) String() string {
	return fmt.Sprintf("%s@%s", k.Name, k.Namespace)
}

// MarshalText marshals the Key.
func (k Key) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText unmarshals the Key.
func (k *Key) UnmarshalText(data []byte) error {
	parts := strings.Split(string(data), "@")
	if len(parts) != 2 {
		return fmt.Errorf("unable to unmarshal Key: %s", string(data))
	}

	k.Name = parts[0]
	k.Namespace = parts[1]

	return nil
}

// UnmarshalJSON implements the `json.Unmarshaler` interface.
// This is a temporary workaround for the bug described in this
// issue: https://github.com/golang/go/issues/38771.
func (k *Key) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return nil
	}

	data = data[1 : len(data)-1]

	return k.UnmarshalText(data)
}

// ServiceTrafficTargetKey references a TrafficTarget applied on a Service.
type ServiceTrafficTargetKey struct {
	Service       Key
	TrafficTarget Key
}

// String stringifies the ServiceTrafficTargetKey.
func (k ServiceTrafficTargetKey) String() string {
	return k.Service.String() + ":" + k.TrafficTarget.String()
}

// MarshalText marshals the ServiceTrafficTargetKey.
func (k ServiceTrafficTargetKey) MarshalText() ([]byte, error) {
	svcKey, err := k.Service.MarshalText()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ServiceTrafficTarget: Service is invalid: %w", err)
	}

	ttKey, err := k.TrafficTarget.MarshalText()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ServiceTrafficTarget: TrafficTarget is invalid: %w", err)
	}

	return []byte(string(svcKey) + ":" + string(ttKey)), nil
}

// UnmarshalText unmarshals the ServiceTrafficTargetKey.
func (k *ServiceTrafficTargetKey) UnmarshalText(data []byte) error {
	parts := strings.Split(string(data), ":")
	if len(parts) != 2 {
		return fmt.Errorf("unable to unmarshal ServiceTrafficTargetKey: %s", string(data))
	}

	if err := k.Service.UnmarshalText([]byte(parts[0])); err != nil {
		return fmt.Errorf("unable to unmarshal ServiceTrafficTargetKey: Service Key is invalid: %w", err)
	}

	if err := k.TrafficTarget.UnmarshalText([]byte(parts[1])); err != nil {
		return fmt.Errorf("unable to unmarshal ServiceTrafficTargetKey: TrafficTarget Key is invalid: %w", err)
	}

	return nil
}

// UnmarshalJSON implements the `json.Unmarshaler` interface.
// This is a temporary workaround for the bug described in this
// issue: https://github.com/golang/go/issues/38771.
func (k *ServiceTrafficTargetKey) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return nil
	}

	data = data[1 : len(data)-1]

	return k.UnmarshalText(data)
}

// Topology holds the graph and represents the different paths a request can follow. Each Pods and services are nodes
// of the graph.
type Topology struct {
	Services              map[Key]*Service                                  `json:"services"`
	Pods                  map[Key]*Pod                                      `json:"pods"`
	ServiceTrafficTargets map[ServiceTrafficTargetKey]*ServiceTrafficTarget `json:"serviceTrafficTargets"`
	TrafficSplits         map[Key]*TrafficSplit                             `json:"trafficSplits"`
}

// NewTopology creates a new Topology.
func NewTopology() *Topology {
	return &Topology{
		Services:              make(map[Key]*Service),
		Pods:                  make(map[Key]*Pod),
		ServiceTrafficTargets: make(map[ServiceTrafficTargetKey]*ServiceTrafficTarget),
		TrafficSplits:         make(map[Key]*TrafficSplit),
	}
}

// Service is a node of the graph representing a kubernetes service.
type Service struct {
	Name        string               `json:"name"`
	Namespace   string               `json:"namespace"`
	Selector    map[string]string    `json:"selector"`
	Annotations map[string]string    `json:"annotations"`
	Ports       []corev1.ServicePort `json:"ports,omitempty"`
	ClusterIP   string               `json:"clusterIp"`
	Pods        []Key                `json:"pods,omitempty"`

	// List of TrafficTargets that are targeting pods which are selected by this service.
	TrafficTargets []ServiceTrafficTargetKey `json:"trafficTargets,omitempty"`
	// List of TrafficSplits that are targeting this service.
	TrafficSplits []Key `json:"trafficSplits,omitempty"`
	// List of TrafficSplit mentioning this service as a backend.
	BackendOf []Key `json:"backendOf,omitempty"`

	Errors []string `json:"errors"`
}

// AddError adds the given error to this Service.
func (s *Service) AddError(err error) {
	s.Errors = append(s.Errors, err.Error())
}

// ServiceTrafficTarget represents a TrafficTarget applied a on Service. TrafficTargets have a Destination service
// account. This service account can be set on many pods, each of them, potentially accessible through different services.
// A ServiceTrafficTarget is a TrafficTarget for a Service which exposes a Pod which has the TrafficTarget Destination
// service-account.
type ServiceTrafficTarget struct {
	Service   Key    `json:"service"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	Sources     []ServiceTrafficTargetSource    `json:"sources,omitempty"`
	Destination ServiceTrafficTargetDestination `json:"destination"`
	Rules       []TrafficSpec                   `json:"rules,omitempty"`

	Errors []string `json:"errors"`
}

// AddError adds the given error to this ServiceTrafficTarget.
func (tt *ServiceTrafficTarget) AddError(err error) {
	tt.Errors = append(tt.Errors, err.Error())
}

// ServiceTrafficTargetSource represents a source of a ServiceTrafficTarget. In the SMI specification, a TrafficTarget
// has a list of sources, each of them being a service-account name. ServiceTrafficTargetSource represents this
// service-account, populated with the pods having this Service.
type ServiceTrafficTargetSource struct {
	ServiceAccount string `json:"serviceAccount"`
	Namespace      string `json:"namespace"`
	Pods           []Key  `json:"pods,omitempty"`
}

// ServiceTrafficTargetDestination represents a destination of a ServiceTrafficTarget. In the SMI specification, a
// TrafficTarget has a destination service-account. ServiceTrafficTargetDestination holds the pods exposed by the
// Service which has this service-account.
type ServiceTrafficTargetDestination struct {
	ServiceAccount string               `json:"serviceAccount"`
	Namespace      string               `json:"namespace"`
	Ports          []corev1.ServicePort `json:"ports,omitempty"`
	Pods           []Key                `json:"pods,omitempty"`
}

// Pod is a node of the graph representing a kubernetes pod.
type Pod struct {
	Name            string                 `json:"name"`
	Namespace       string                 `json:"namespace"`
	ServiceAccount  string                 `json:"serviceAccount"`
	OwnerReferences []v1.OwnerReference    `json:"ownerReferences,omitempty"`
	ContainerPorts  []corev1.ContainerPort `json:"containerPorts,omitempty"`
	IP              string                 `json:"ip"`

	SourceOf      []ServiceTrafficTargetKey `json:"sourceOf,omitempty"`
	DestinationOf []ServiceTrafficTargetKey `json:"destinationOf,omitempty"`
}

// TrafficSplit represents a TrafficSplit applied on a Service.
type TrafficSplit struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	Service  Key                   `json:"service"`
	Backends []TrafficSplitBackend `json:"backends,omitempty"`
	Rules    []TrafficSpec         `json:"rules,omitempty"`

	// List of Pods that are explicitly allowed to pass through the TrafficSplit.
	Incoming []Key `json:"incoming,omitempty"`

	Errors []string `json:"errors"`
}

// TrafficSpec represents a Spec which can be used for restricting access to a route in a TrafficTarget or a TrafficSplit.
type TrafficSpec struct {
	HTTPRouteGroup *specs.HTTPRouteGroup `json:"httpRouteGroup,omitempty"`
	TCPRoute       *specs.TCPRoute       `json:"tcpRoute,omitempty"`
}

// AddError adds the given error to this TrafficSplit.
func (ts *TrafficSplit) AddError(err error) {
	ts.Errors = append(ts.Errors, err.Error())
}

// TrafficSplitBackend is a backend of a TrafficSplit.
type TrafficSplitBackend struct {
	Weight  int `json:"weight"`
	Service Key `json:"service"`
}

// ResolveServicePort resolves the given service port against the given container port list, as described in the
// Kubernetes documentation, and returns true if it has been successfully resolved, false otherwise.
//
// The Kubernetes documentation says: Port definitions in Pods have names, and you can reference these names in the
// targetPort attribute of a Service. This works even if there is a mixture of Pods in the Service using a single
// configured name, with the same network protocol available via different port numbers.
func ResolveServicePort(svcPort corev1.ServicePort, containerPorts []corev1.ContainerPort) (int32, bool) {
	if svcPort.TargetPort.Type == intstr.Int {
		return svcPort.TargetPort.IntVal, true
	}

	for _, containerPort := range containerPorts {
		if svcPort.TargetPort.StrVal == containerPort.Name && svcPort.Protocol == containerPort.Protocol {
			return containerPort.ContainerPort, true
		}
	}

	return 0, false
}
