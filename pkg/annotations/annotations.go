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
	// SchemeH2c h2c scheme.
	SchemeH2c string = "h2c"
	// SchemeHTTPS HTTPS scheme.
	SchemeHTTPS string = "https"
)

const (
	baseAnnotation        = "maesh.containo.us/"
	annotationServiceType = baseAnnotation + "traffic-type"
	annotationScheme      = baseAnnotation + "scheme"
)

func GetTrafficType(defaultTrafficType string, annotations map[string]string) (string, error) {
	trafficType, ok := annotations[annotationServiceType]

	if !ok {
		return defaultTrafficType, nil
	}

	if trafficType != ServiceTypeHTTP && trafficType != ServiceTypeTCP && trafficType != ServiceTypeUDP {
		return "", fmt.Errorf("traffic-type annotation references an unsupported traffic type %q", trafficType)
	}

	return trafficType, nil
}

func GetScheme(annotations map[string]string) (string, error) {
	scheme, ok := annotations[annotationScheme]

	if !ok {
		return SchemeHTTP, nil
	}

	if scheme != SchemeHTTP && scheme != SchemeH2c && scheme != SchemeHTTPS {
		return "", fmt.Errorf("scheme annotation references an unknown scheme %q", scheme)
	}

	return scheme, nil
}
