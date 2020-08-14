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
	annotationServiceType              = "traffic-type"
	annotationScheme                   = "scheme"
	annotationRetryAttempts            = "retry-attempts"
	annotationCircuitBreakerExpression = "circuit-breaker-expression"
	annotationRateLimitAverage         = "ratelimit-average"
	annotationRateLimitBurst           = "ratelimit-burst"
)

// ErrNotFound indicates that the annotation hasn't been found.
var ErrNotFound = errors.New("annotation not found")

// GetTrafficType returns the value of the traffic-type annotation.
func GetTrafficType(defaultTrafficType string, annotations map[string]string) (string, error) {
	trafficType, exists := getAnnotation(annotations, annotationServiceType)
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

// GetScheme returns the value of the scheme annotation.
func GetScheme(annotations map[string]string) (string, error) {
	scheme, exists := getAnnotation(annotations, annotationScheme)
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
	retryAttempts, exists := getAnnotation(annotations, annotationRetryAttempts)
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
	circuitBreakerExpression, exists := getAnnotation(annotations, annotationCircuitBreakerExpression)
	if !exists {
		return "", ErrNotFound
	}

	return circuitBreakerExpression, nil
}

// GetRateLimitBurst returns the value of the rate-limit-burst annotation.
func GetRateLimitBurst(annotations map[string]string) (int, error) {
	rateLimitBurst, exists := getAnnotation(annotations, annotationRateLimitBurst)
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
	rateLimitAverage, ok := getAnnotation(annotations, annotationRateLimitAverage)
	if !ok {
		return 0, ErrNotFound
	}

	average, err := strconv.Atoi(rateLimitAverage)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q: %w", annotationRateLimitAverage, err)
	}

	return average, nil
}

// getAnnotation returns the value of the annotation with the given name and a boolean evaluating to true if the
// annotation has been found, false otherwise. This function will try to resolve the annotation with the traefik mesh
// domain prefix and fallback to the deprecated maesh domain prefix if not found.
func getAnnotation(annotations map[string]string, name string) (string, bool) {
	value, exists := annotations["mesh.traefik.io/"+name]
	if !exists {
		value, exists = annotations["maesh.containo.us/"+name]
	}

	return value, exists
}
