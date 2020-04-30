package annotations

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
)

const (
	annotationRetryAttempts            = baseAnnotation + "retry-attempts"
	annotationCircuitBreakerExpression = baseAnnotation + "circuit-breaker-expression"
	annotationRateLimitAverage         = baseAnnotation + "ratelimit-average"
	annotationRateLimitBurst           = baseAnnotation + "ratelimit-burst"
)

// BuildMiddleware builds a middleware from the given annotations
func BuildMiddleware(annotations map[string]string) (*dynamic.Middleware, error) {
	var middleware dynamic.Middleware

	// Build circuit-breaker middleware.
	if circuitBreakerExpression, ok := annotations[annotationCircuitBreakerExpression]; ok {
		middleware.CircuitBreaker = &dynamic.CircuitBreaker{Expression: circuitBreakerExpression}
	}

	// Build retry middleware.
	if retryAttempts, ok := annotations[annotationRetryAttempts]; ok {
		attempts, err := strconv.Atoi(retryAttempts)
		if err != nil {
			return nil, fmt.Errorf("unable to build retry middleware, %q annotation is invalid: %w", annotationRetryAttempts, err)
		}

		middleware.Retry = &dynamic.Retry{Attempts: attempts}
	}

	// Build rate-limit middleware.
	rateLimitAverage, hasRateLimitAverage := annotations[annotationRateLimitAverage]
	rateLimitBurst, hasRateLimitBurst := annotations[annotationRateLimitBurst]

	if hasRateLimitAverage && hasRateLimitBurst {
		average, err := strconv.Atoi(rateLimitAverage)
		if err != nil {
			return nil, fmt.Errorf("unable to build rate-limit middleware, %q annotation is invalid: %w", annotationRateLimitAverage, err)
		}

		burst, err := strconv.Atoi(rateLimitBurst)
		if err != nil {
			return nil, fmt.Errorf("unable to build rate-limit middleware, %q annotation is invalid: %w", annotationRateLimitBurst, err)
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
