package annotations

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	// ServiceTypeHTTP HTTP service type.
	ServiceTypeHTTP string = "http"
	// ServiceTypeTCP TCP service type.
	ServiceTypeTCP string = "tcp"
	// ServiceTypeUDP UDP service type.
	ServiceTypeUDP string = "udp"

	// SchemeHTTP HTTP scheme.
	SchemeHTTP string = "http"
	// SchemeH2C h2c scheme.
	SchemeH2C string = "h2c"
	// SchemeHTTPS HTTPS scheme.
	SchemeHTTPS string = "https"
)

const (
	baseAnnotation                     = "mesh.traefik.io/"
	annotationServiceType              = baseAnnotation + "traffic-type"
	annotationScheme                   = baseAnnotation + "scheme"
	annotationRetryAttempts            = baseAnnotation + "retry-attempts"
	annotationCircuitBreakerExpression = baseAnnotation + "circuit-breaker-expression"
	annotationRateLimitAverage         = baseAnnotation + "ratelimit-average"
	annotationRateLimitBurst           = baseAnnotation + "ratelimit-burst"
)

// ErrNotFound indicates that the annotation hasn't been found.
var ErrNotFound = errors.New("annotation not found")

// GetTrafficType returns the value of the traffic-type annotation.
func GetTrafficType(defaultTrafficType string, annotations map[string]string) (string, error) {
	trafficType, exists := annotations[annotationServiceType]
	if !exists {
		return defaultTrafficType, nil
	}

	switch trafficType {
	case ServiceTypeHTTP:
	case ServiceTypeTCP:
	case ServiceTypeUDP:
	default:
		return trafficType, fmt.Errorf("unsupported traffic type %q: %q", annotationServiceType, trafficType)
	}

	return trafficType, nil
}

// SetTrafficType sets the traffic-type annotation to the given value.
func SetTrafficType(trafficType string, annotations map[string]string) {
	annotations[annotationServiceType] = trafficType
}

// GetScheme returns the value of the scheme annotation.
func GetScheme(annotations map[string]string) (string, error) {
	scheme, exists := annotations[annotationScheme]
	if !exists {
		return SchemeHTTP, nil
	}

	switch scheme {
	case SchemeHTTP:
	case SchemeH2C:
	case SchemeHTTPS:
	default:
		return scheme, fmt.Errorf("unsupported scheme %q: %q", annotationScheme, scheme)
	}

	return scheme, nil
}

// GetRetryAttempts returns the value of the retry-attempts annotation.
func GetRetryAttempts(annotations map[string]string) (int, error) {
	retryAttempts, exists := annotations[annotationRetryAttempts]
	if !exists {
		return 0, ErrNotFound
	}

	attempts, err := strconv.Atoi(retryAttempts)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q: %w", annotationRetryAttempts, err)
	}

	return attempts, nil
}

// GetCircuitBreakerExpression returns the value of the circuit-breaker-expression annotation.
func GetCircuitBreakerExpression(annotations map[string]string) (string, error) {
	circuitBreakerExpression, exists := annotations[annotationCircuitBreakerExpression]
	if !exists {
		return "", ErrNotFound
	}

	return circuitBreakerExpression, nil
}

// GetRateLimitBurst returns the value of the rate-limit-burst annotation.
func GetRateLimitBurst(annotations map[string]string) (int, error) {
	rateLimitBurst, exists := annotations[annotationRateLimitBurst]
	if !exists {
		return 0, ErrNotFound
	}

	burst, err := strconv.Atoi(rateLimitBurst)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q: %w", annotationRateLimitBurst, err)
	}

	return burst, nil
}

// GetRateLimitAverage returns the value of the rate-limit-average annotation.
func GetRateLimitAverage(annotations map[string]string) (int, error) {
	rateLimitAverage, ok := annotations[annotationRateLimitAverage]
	if !ok {
		return 0, ErrNotFound
	}

	average, err := strconv.Atoi(rateLimitAverage)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q: %w", annotationRateLimitAverage, err)
	}

	return average, nil
}
