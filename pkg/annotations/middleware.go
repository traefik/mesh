package annotations

import (
	"errors"
	"fmt"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
)

type middlewareBuilder func(middleware *dynamic.Middleware, annotations map[string]string) (bool, error)

// BuildMiddleware builds a middleware from the given annotations
func BuildMiddleware(annotations map[string]string) (*dynamic.Middleware, error) {
	builders := []middlewareBuilder{
		buildRetryMiddleware,
		buildRateLimitMiddleware,
		buildCircuitBreakerMiddleware,
	}

	var (
		middleware        dynamic.Middleware
		middlewareCounter int
	)

	for _, builder := range builders {
		ok, err := builder(&middleware, annotations)
		if err != nil {
			return nil, err
		}

		if ok {
			middlewareCounter++
		}
	}

	if middlewareCounter == 0 {
		return nil, nil
	}

	return &middleware, nil
}

func buildRetryMiddleware(middleware *dynamic.Middleware, annotations map[string]string) (bool, error) {
	retryAttempts, errRetryAttempts := GetRetryAttempts(annotations)
	if errRetryAttempts != nil {
		if errRetryAttempts == ErrNotFound {
			return false, nil
		}

		return false, fmt.Errorf("unable to build retry middleware: %w", errRetryAttempts)
	}

	middleware.Retry = &dynamic.Retry{Attempts: retryAttempts}

	return true, nil
}

func buildRateLimitMiddleware(middleware *dynamic.Middleware, annotations map[string]string) (bool, error) {
	var (
		rateLimitBurst   int
		rateLimitAverage int
		err              error
	)

	rateLimitBurst, err = GetRateLimitBurst(annotations)
	if err == ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("unable to build rate-limit middleware: %w", err)
	}

	rateLimitAverage, err = GetRateLimitAverage(annotations)
	if err == ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("unable to build rate-limit middleware: %w", err)
	}

	if rateLimitBurst <= 0 || rateLimitAverage <= 0 {
		return false, errors.New("unable to build rate-limit middleware: burst and average must be greater than 0")
	}

	middleware.RateLimit = &dynamic.RateLimit{
		Burst:   int64(rateLimitBurst),
		Average: int64(rateLimitAverage),
	}

	return true, nil
}

func buildCircuitBreakerMiddleware(middleware *dynamic.Middleware, annotations map[string]string) (bool, error) {
	circuitBreakerExpression, errCircuitBreakerExpression := GetCircuitBreakerExpression(annotations)
	if errCircuitBreakerExpression != nil {
		if errCircuitBreakerExpression == ErrNotFound {
			return false, nil
		}

		return false, fmt.Errorf("unable to build circuit-breaker middleware: %w", errCircuitBreakerExpression)
	}

	middleware.CircuitBreaker = &dynamic.CircuitBreaker{Expression: circuitBreakerExpression}

	return true, nil
}
