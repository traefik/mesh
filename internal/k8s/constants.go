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
	// AnnotationRetryAttempts retry attempts annotation.
	AnnotationRetryAttempts = baseAnnotation + "retry-attempts"
	// AnnotationCircuitBreakerExpression circuit breaker expression annotation.
	AnnotationCircuitBreakerExpression = baseAnnotation + "circuit-breaker-expression"

	// ServiceTypeHTTP HTTP service type.
	ServiceTypeHTTP string = "http"
	// ServiceTypeTCP TCP service type.
	ServiceTypeTCP string = "tcp"

	// BlockAllMiddlewareKey block all middleware name.
	BlockAllMiddlewareKey string = "smi-block-all-middleware"

	// TCPStateConfigMapName TCP config map name.
	TCPStateConfigMapName string = "tcp-state-table"
)
