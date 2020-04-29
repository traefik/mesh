package k8s

import (
	"time"
)

const (
	// ResyncPeriod set the resync period.
	ResyncPeriod          = 5 * time.Minute
	baseAnnotation string = "maesh.containo.us/"

	// CoreObjectKinds is a filter for objects to process by the core client.
	CoreObjectKinds = "Deployment|Endpoints|Service|Ingress|Secret|Namespace|Pod|ConfigMap"
	// AccessObjectKinds is a filter for objects to process by the access client.
	AccessObjectKinds = "TrafficTarget"
	// SpecsObjectKinds is a filter for objects to process by the specs client.
	SpecsObjectKinds = "HTTPRouteGroup|TCPRoute"
	// SplitObjectKinds is a filter for objects to process by the split client.
	SplitObjectKinds = "TrafficSplit"

	// AnnotationServiceType service type annotation.
	AnnotationServiceType = baseAnnotation + "traffic-type"
	// AnnotationScheme scheme.
	AnnotationScheme = baseAnnotation + "scheme"
	// AnnotationRetryAttempts retry attempts annotation.
	AnnotationRetryAttempts = baseAnnotation + "retry-attempts"
	// AnnotationCircuitBreakerExpression circuit breaker expression annotation.
	AnnotationCircuitBreakerExpression = baseAnnotation + "circuit-breaker-expression"
	// AnnotationRateLimitAverage sets the average value for rate limiting.
	AnnotationRateLimitAverage = baseAnnotation + "ratelimit-average"
	// AnnotationRateLimitBurst sets the burst value for rate limiting.
	AnnotationRateLimitBurst = baseAnnotation + "ratelimit-burst"

	// ServiceTypeHTTP HTTP service type.
	ServiceTypeHTTP string = "http"
	// ServiceTypeTCP TCP service type.
	ServiceTypeTCP string = "tcp"
	// ServiceTypeUDP UDP service type.
	ServiceTypeUDP string = "udp"

	// SchemeHTTP HTTP scheme.
	SchemeHTTP string = "http"
	// SchemeH2c h2c scheme.
	SchemeH2c string = "h2c"
	// SchemeHTTPS HTTPS scheme.
	SchemeHTTPS string = "https"

	// TCPStateConfigMapName TCP config map name.
	TCPStateConfigMapName string = "tcp-state-table"
	// UDPStateConfigMapName UDP config map name.
	UDPStateConfigMapName string = "udp-state-table"

	// ConfigMessageChanRebuild rebuild.
	ConfigMessageChanRebuild string = "rebuild"
	// ConfigMessageChanForce force.
	ConfigMessageChanForce string = "force"
)
