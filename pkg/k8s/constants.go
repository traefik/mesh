package k8s

import (
	"time"
)

const (
	// ResyncPeriod set the resync period.
	ResyncPeriod = 5 * time.Minute

	// CoreObjectKinds is a filter for objects to process by the core client.
	CoreObjectKinds = "Deployment|Endpoints|Service|Ingress|Secret|Namespace|Pod|ConfigMap"
	// AccessObjectKinds is a filter for objects to process by the access client.
	AccessObjectKinds = "TrafficTarget"
	// SpecsObjectKinds is a filter for objects to process by the specs client.
	SpecsObjectKinds = "HTTPRouteGroup|TCPRoute"
	// SplitObjectKinds is a filter for objects to process by the split client.
	SplitObjectKinds = "TrafficSplit"
)
