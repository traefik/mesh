package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// LabelName is used for specifying the name of the app.
	LabelName = "app.kubernetes.io/name"
	// LabelComponent is used for specifiying a specific component of the app.
	LabelComponent = "app.kubernetes.io/component"
	// LabelPartOf is used for specifying the name of a higher level app it is part of.
	LabelPartOf = "app.kubernetes.io/part-of"

	// AppName is the name of the app.
	AppName = "traefik-mesh"

	// ComponentProxy is component of type proxy.
	ComponentProxy = "proxy"
	// ComponentShadowService is component of type shadow-service.
	ComponentShadowService = "shadow-service"
)

// ShadowServiceLabels are the labels of a shadow service.
var ShadowServiceLabels = map[string]string{
	LabelName:      AppName,
	LabelComponent: ComponentShadowService,
	LabelPartOf:    AppName,
}

// ProxyLabels are the labels of a proxy.
var ProxyLabels = map[string]string{
	LabelName:      AppName,
	LabelComponent: ComponentProxy,
	LabelPartOf:    AppName,
}

// ShadowServiceSelector creates a label selector for shadow services.
func ShadowServiceSelector() labels.Selector {
	selector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: ShadowServiceLabels,
	})

	return selector
}

// ProxySelector creates a label selector for proxies.
func ProxySelector() labels.Selector {
	selector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: ProxyLabels,
	})

	return selector
}
