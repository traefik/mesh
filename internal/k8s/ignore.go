package k8s

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// FieldSelector returns the field selectors query representing the ignored namespace and services.
func (i *IgnoreWrapper) FieldSelector() string {
	var selectors []string

	for _, n := range i.Namespaces {
		selectors = append(selectors, "metadata.namespace!="+n)
	}

	// We are loosing the filter by specifng namespace and service name here, but not sure if it's needed.
	for _, n := range i.Services {
		selectors = append(selectors, "metadata.name!="+n.Name)
	}

	return strings.Join(selectors, ",")
}

// LabelSelector returns the label selector representing the ignored apps.
func (i *IgnoreWrapper) LabelSelector() string {
	var selectors []string

	for _, a := range i.Apps {
		selectors = append(selectors, "app!="+a)
	}

	return strings.Join(selectors, ",")
}

// IsIgnored returns if the object events should be ignored.
func (i *IgnoreWrapper) IsIgnored(obj metav1.ObjectMeta) bool {
	// Is the object's namespace ignored?
	if i.Namespaces.Contains(obj.GetNamespace()) {
		return true
	}

	// Is the object explicitly ignored?
	if i.Services.Contains(obj.GetName(), obj.GetNamespace()) {
		return true
	}

	// Is the app ignored ?
	if contains(i.Apps, obj.GetLabels()["app"]) {
		return true
	}

	return false
}

// IsIgnoredNamespace returns if the service's events should be ignored.
func (i *IgnoreWrapper) IsIgnoredNamespace(namespace string) bool {
	return i.Namespaces.Contains(namespace)
}
