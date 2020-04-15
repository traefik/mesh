package provider

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
)

// MiddlewareBuilder builds middlewares of a service.
type MiddlewareBuilder func(svc *topology.Service) (*dynamic.Middleware, error)

// Build builds middlewares of the given service using annotations.
func buildMiddlewareFromAnnotations(svc *topology.Service) (*dynamic.Middleware, error) {
	var middleware dynamic.Middleware

	// Build circuit-breaker middleware.
	if circuitBreakerExpression, ok := svc.Annotations[k8s.AnnotationCircuitBreakerExpression]; ok {
		middleware.CircuitBreaker = &dynamic.CircuitBreaker{Expression: circuitBreakerExpression}
	}

	// Build retry middleware.
	if retryAttempts, ok := svc.Annotations[k8s.AnnotationRetryAttempts]; ok {
		attempts, err := strconv.Atoi(retryAttempts)
		if err != nil {
			return nil, fmt.Errorf("unable to build retry middleware, %q annotation is invalid: %w", k8s.AnnotationRetryAttempts, err)
		}

		middleware.Retry = &dynamic.Retry{Attempts: attempts}
	}

	// Build rate-limit middleware.
	rateLimitAverage, hasRateLimitAverage := svc.Annotations[k8s.AnnotationRateLimitAverage]
	rateLimitBurst, hasRateLimitBurst := svc.Annotations[k8s.AnnotationRateLimitBurst]

	if hasRateLimitAverage && hasRateLimitBurst {
		average, err := strconv.Atoi(rateLimitAverage)
		if err != nil {
			return nil, fmt.Errorf("unable to build rate-limit middleware, %q annotation is invalid: %w", k8s.AnnotationRateLimitAverage, err)
		}

		burst, err := strconv.Atoi(rateLimitBurst)
		if err != nil {
			return nil, fmt.Errorf("unable to build rate-limit middleware, %q annotation is invalid: %w", k8s.AnnotationRateLimitBurst, err)
		}

		if burst <= 0 || average <= 0 {
			return nil, errors.New("unable to build rate-limit middleware, burst and average must be greater than 0")
		}

		middleware.RateLimit = &dynamic.RateLimit{
			Average: int64(average),
			Burst:   int64(burst),
		}
	}

	if middleware.CircuitBreaker == nil && middleware.Retry == nil && middleware.RateLimit == nil {
		return nil, nil
	}

	return &middleware, nil
}
