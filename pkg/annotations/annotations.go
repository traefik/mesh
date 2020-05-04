package annotations

import (
	"fmt"
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
	baseAnnotation        = "maesh.containo.us/"
	annotationServiceType = baseAnnotation + "traffic-type"
	annotationScheme      = baseAnnotation + "scheme"
)

// GetTrafficType returns the value of the traffic-type annotation.
func GetTrafficType(defaultTrafficType string, annotations map[string]string) (string, error) {
	trafficType, ok := annotations[annotationServiceType]
	if !ok {
		return defaultTrafficType, nil
	}

	switch trafficType {
	case ServiceTypeHTTP:
	case ServiceTypeTCP:
	case ServiceTypeUDP:
	default:
		return trafficType, fmt.Errorf("traffic-type annotation references an unsupported traffic type %q", trafficType)
	}

	return trafficType, nil
}

// GetScheme returns the value of the scheme annotation.
func GetScheme(annotations map[string]string) (string, error) {
	scheme, ok := annotations[annotationScheme]
	if !ok {
		return SchemeHTTP, nil
	}

	switch scheme {
	case SchemeHTTP:
	case SchemeH2C:
	case SchemeHTTPS:
	default:
		return scheme, fmt.Errorf("scheme annotation references an unknown scheme %q", scheme)
	}

	return scheme, nil
}

//func GetRetryAttempts(annotations map[string]string)
