package k8s

import (
	"time"
)

const (
	// ResyncPeriod set the resync period.
	ResyncPeriod          = 5 * time.Minute
	baseAnnotation string = "maesh.containo.us/"

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

	// SchemeHTTP HTTP scheme.
	SchemeHTTP string = "http"
	// SchemeH2c h2c scheme.
	SchemeH2c string = "h2c"

	// BlockAllMiddlewareKey block all middleware name.
	BlockAllMiddlewareKey string = "smi-block-all-middleware"

	// TCPStateConfigMapName TCP config map name.
	TCPStateConfigMapName string = "tcp-state-table"

	// ConfigMessageChanRebuild rebuild.
	ConfigMessageChanRebuild string = "rebuild"
	// ConfigMessageChanForce force.
	ConfigMessageChanForce string = "force"
)
