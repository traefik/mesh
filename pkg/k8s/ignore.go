package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// IgnoreWrapper holds namespaces and services to ignore.
type IgnoreWrapper struct {
	Namespaces Namespaces
	Services   Services
	Apps       []string
}

// NewIgnored returns a new IgnoreWrapper.
func NewIgnored() IgnoreWrapper {
	return IgnoreWrapper{
		Namespaces: Namespaces{},
		Services:   Services{},
		Apps:       []string{},
	}
}

// AddIgnoredNamespace adds a namespace to the list of ignored namespaces.
func (i *IgnoreWrapper) AddIgnoredNamespace(namespace string) {
	i.Namespaces = append(i.Namespaces, namespace)
}

// AddIgnoredService adds a service to the list of ignored services.
func (i *IgnoreWrapper) AddIgnoredService(serviceName, serviceNamespace string) {
	i.Services = append(i.Services, Service{Name: serviceName, Namespace: serviceNamespace})
}

// AddIgnoredApps add an app to the list of ignored apps.
func (i *IgnoreWrapper) AddIgnoredApps(app ...string) {
	i.Apps = append(i.Apps, app...)
}

// IsIgnored returns if the object events should be ignored.
func (i *IgnoreWrapper) IsIgnored(obj interface{}) bool {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return false
	}

	pMeta := meta.AsPartialObjectMetadata(accessor)

	// Is the object's namespace ignored?
	if i.Namespaces.Contains(pMeta.GetNamespace()) {
		return true
	}

	// Is the app ignored?
	if contains(i.Apps, pMeta.GetLabels()["app"]) {
		return true
	}

	if svc, ok := obj.(*corev1.Service); ok {
		// Is the object explicitly ignored?
		if i.Services.Contains(pMeta.GetName(), pMeta.GetNamespace()) {
			return true
		}

		// Ignore ExternalName services.
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			return true
		}
	}

	return false
}
