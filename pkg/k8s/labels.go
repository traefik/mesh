package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// LabelName is used for specifying the name of the app.
	LabelName = "app.kubernetes.io/name"
	// LabelComponent is used for specifying a specific component of the app.
	LabelComponent = "app.kubernetes.io/component"
	// LabelPartOf is used for specifying the name of a higher level app it is part of.
	LabelPartOf = "app.kubernetes.io/part-of"
	// LabelServiceName is the name of the label for storing the name of the source service for a shadow service.
	LabelServiceName = "mesh.traefik.io/service-name"
	// LabelServiceNamespace is the name of the label for storing the namespace of the source service for a shadow service.
	LabelServiceNamespace = "mesh.traefik.io/service-namespace"

	// AppName is the name of the app.
	AppName = "traefik-mesh"

	// ComponentProxy is component of type proxy.
	ComponentProxy = "proxy"
	// ComponentShadowService is component of type shadow-service.
	ComponentShadowService = "shadow-service"
)

// ShadowServiceLabels returns the labels of a shadow service.
func ShadowServiceLabels() map[string]string {
	return map[string]string{
		LabelName:      AppName,
		LabelComponent: ComponentShadowService,
		LabelPartOf:    AppName,
	}
}

// ProxyLabels returns the labels of a proxy.
func ProxyLabels() map[string]string {
	return map[string]string{
		LabelName:      AppName,
		LabelComponent: ComponentProxy,
		LabelPartOf:    AppName,
	}
}

// ShadowServiceSelector creates a label selector for shadow services.
func ShadowServiceSelector() labels.Selector {
	selector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: ShadowServiceLabels(),
	})

	return selector
}

// ProxySelector creates a label selector for proxies.
func ProxySelector() labels.Selector {
	selector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: ProxyLabels(),
	})

	return selector
}
