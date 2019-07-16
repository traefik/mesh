package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IgnoreWrapper holds namespaces and services to ignore.
type IgnoreWrapper struct {
	Namespaces Namespaces
	Services   Services
}

// Ignored returns if the selected name or namespace combo should be ignored.
func (i *IgnoreWrapper) Ignored(name, namespace string) bool {
	if i.Namespaces.Contains(namespace) {
		return true
	}

	if i.Services.Contains(name, namespace) {
		return true
	}
	return false
}

// WithoutMesh returns an IgnoreWrapper without the mesh namespace.
func (i *IgnoreWrapper) WithoutMesh() IgnoreWrapper {
	namespaces := Namespaces{}
	for _, ns := range i.Namespaces {
		if ns != MeshNamespace {
			namespaces = append(namespaces, ns)
		}
	}

	return IgnoreWrapper{
		Namespaces: namespaces,
		Services:   i.Services,
	}
}

// NewIgnored returns a new IgnoreWrapper.
func NewIgnored() IgnoreWrapper {
	ignoredNamespaces := Namespaces{metav1.NamespaceSystem, MeshNamespace}
	ignoredServices := Services{
		{
			Name:      "kubernetes",
			Namespace: metav1.NamespaceDefault,
		},
	}

	return IgnoreWrapper{
		Namespaces: ignoredNamespaces,
		Services:   ignoredServices,
	}

}
