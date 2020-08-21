package k8s

import (
	"time"
)

const (
	// ResyncPeriod set the resync period.
	ResyncPeriod = 5 * time.Minute

	// TrafficSplitObjectKind is the name of an SMI object of kind TrafficSplit.
	TrafficSplitObjectKind = "TrafficSplit"
	// TrafficTargetObjectKind is the name of an SMI object of kind TrafficTarget.
	TrafficTargetObjectKind = "TrafficTarget"
	// HTTPRouteGroupObjectKind is the name of an SMI object of kind HTTPRouteGroup.
	HTTPRouteGroupObjectKind = "HTTPRouteGroup"
	// TCPRouteObjectKind is the name of an SMI object of kind TCPRoute.
	TCPRouteObjectKind = "TCPRoute"

	// CoreObjectKinds is a filter for objects to process by the core client.
	CoreObjectKinds = "Deployment|Endpoints|Service|Ingress|Secret|Namespace|Pod|ConfigMap"
	// AccessObjectKinds is a filter for objects to process by the access client.
	AccessObjectKinds = TrafficTargetObjectKind
	// SpecsObjectKinds is a filter for objects to process by the specs client.
	SpecsObjectKinds = HTTPRouteGroupObjectKind + "|" + TCPRouteObjectKind
	// SplitObjectKinds is a filter for objects to process by the split client.
	SplitObjectKinds = TrafficSplitObjectKind
)
