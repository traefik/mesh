package annotations

import (
	"errors"
	"fmt"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
)

type middlewareBuilder func(annotations map[string]string) (*dynamic.Middleware, string, error)

// BuildMiddlewares builds middlewares from the given annotations.
func BuildMiddlewares(annotations map[string]string) (map[string]*dynamic.Middleware, error) {

	builders := []middlewareBuilder{
		buildRetryMiddleware,
		buildRateLimitMiddleware,
		buildCircuitBreakerMiddleware,
	}

	middlewares := map[string]*dynamic.Middleware{}

	for _, builder := range builders {
		middleware, name, err := builder(annotations)
		if err != nil {
			return nil, err
		}

		if middleware != nil {
			middlewares[name] = middleware
		}
	}

	return middlewares, nil
}

func buildRetryMiddleware(annotations map[string]string) (*dynamic.Middleware, string, error) {
	retryAttempts, errRetryAttempts := GetRetryAttempts(annotations)
	if errRetryAttempts != nil {
		if errRetryAttempts == ErrNotFound {
			return nil, "", nil
		}

		return nil, "", fmt.Errorf("unable to build retry middleware: %w", errRetryAttempts)
	}

	middleware := &dynamic.Middleware{
		Retry: &dynamic.Retry{Attempts: retryAttempts},
	}

	return middleware, "retry", nil
}

func buildRateLimitMiddleware(annotations map[string]string) (*dynamic.Middleware, string, error) {
	var (
		rateLimitBurst   int
		rateLimitAverage int
		err              error
	)

	rateLimitBurst, err = GetRateLimitBurst(annotations)
	if err == ErrNotFound {
		return nil, "", nil
	} else if err != nil {
		return nil, "", fmt.Errorf("unable to build rate-limit middleware: %w", err)
	}

	rateLimitAverage, err = GetRateLimitAverage(annotations)
	if err == ErrNotFound {
		return nil, "", nil
	} else if err != nil {
		return nil, "", fmt.Errorf("unable to build rate-limit middleware: %w", err)
	}

	if rateLimitBurst <= 0 || rateLimitAverage <= 0 {
		return nil, "", errors.New("unable to build rate-limit middleware: burst and average must be greater than 0")
	}

	middleware := &dynamic.Middleware{
		RateLimit: &dynamic.RateLimit{
			Burst:   int64(rateLimitBurst),
			Average: int64(rateLimitAverage),
		},
	}

	return middleware, "rate-limit", nil
}

func buildCircuitBreakerMiddleware(annotations map[string]string) (*dynamic.Middleware, string, error) {
	circuitBreakerExpression, errCircuitBreakerExpression := GetCircuitBreakerExpression(annotations)
	if errCircuitBreakerExpression != nil {
		if errCircuitBreakerExpression == ErrNotFound {
			return nil, "", nil
		}

		return nil, "", fmt.Errorf("unable to build circuit-breaker middleware: %w", errCircuitBreakerExpression)
	}

	middleware := &dynamic.Middleware{
		CircuitBreaker: &dynamic.CircuitBreaker{
			Expression: circuitBreakerExpression,
		},
	}

	return middleware, "circuit-breaker", nil
}
