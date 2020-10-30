package annotations

import (
	"errors"
	"fmt"

	"github.com/traefik/traefik/v2/pkg/config/dynamic"
)

type middlewareBuilder func(annotations map[string]string) (middleware *dynamic.Middleware, name string, err error)

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

func buildRetryMiddleware(annotations map[string]string) (middleware *dynamic.Middleware, name string, err error) {
	var retryAttempts int

	retryAttempts, err = GetRetryAttempts(annotations)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, "", nil
		}

		return nil, "", fmt.Errorf("unable to build retry middleware: %w", err)
	}

	name = "retry"
	middleware = &dynamic.Middleware{
		Retry: &dynamic.Retry{Attempts: retryAttempts},
	}

	return middleware, name, nil
}

func buildRateLimitMiddleware(annotations map[string]string) (middleware *dynamic.Middleware, name string, err error) {
	var (
		rateLimitBurst   int
		rateLimitAverage int
	)

	rateLimitBurst, err = GetRateLimitBurst(annotations)
	if errors.Is(err, ErrNotFound) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", fmt.Errorf("unable to build rate-limit middleware: %w", err)
	}

	rateLimitAverage, err = GetRateLimitAverage(annotations)
	if errors.Is(err, ErrNotFound) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", fmt.Errorf("unable to build rate-limit middleware: %w", err)
	}

	if rateLimitBurst <= 0 || rateLimitAverage <= 0 {
		return nil, "", errors.New("unable to build rate-limit middleware: burst and average must be greater than 0")
	}

	name = "rate-limit"
	middleware = &dynamic.Middleware{
		RateLimit: &dynamic.RateLimit{
			Burst:   int64(rateLimitBurst),
			Average: int64(rateLimitAverage),
		},
	}

	return middleware, name, nil
}

func buildCircuitBreakerMiddleware(annotations map[string]string) (middleware *dynamic.Middleware, name string, err error) {
	var circuitBreakerExpression string

	circuitBreakerExpression, err = GetCircuitBreakerExpression(annotations)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, "", nil
		}

		return nil, "", fmt.Errorf("unable to build circuit-breaker middleware: %w", err)
	}

	name = "circuit-breaker"
	middleware = &dynamic.Middleware{
		CircuitBreaker: &dynamic.CircuitBreaker{
			Expression: circuitBreakerExpression,
		},
	}

	return middleware, name, nil
}
