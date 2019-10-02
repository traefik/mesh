package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IgnoreWrapper holds namespaces and services to ignore.
type IgnoreWrapper struct {
	Namespaces    Namespaces
	Services      Services
	MeshNamespace string
}

// Ignored returns if the selected name or namespace combo should be ignored.
func (i *IgnoreWrapper) Ignored(name, namespace string) bool {
	if i.Namespaces.Contains(namespace) {
		return true
	}

	if i.Services.Contains(name, namespace) {
		return true
	}

	if i.MeshNamespace != "" && namespace == i.MeshNamespace {
		return true
	}

	return false
}

// WithoutMesh returns an IgnoreWrapper without the mesh namespace.
func (i *IgnoreWrapper) WithoutMesh() IgnoreWrapper {
	return IgnoreWrapper{
		Namespaces: i.Namespaces,
		Services:   i.Services,
	}
}

// NewIgnored returns a new IgnoreWrapper.
func NewIgnored(meshNamespace string, namespacesIgnore []string) IgnoreWrapper {
	ignoredNamespaces := Namespaces{metav1.NamespaceSystem}

	for _, ns := range namespacesIgnore {
		if !ignoredNamespaces.Contains(ns) {
			ignoredNamespaces = append(ignoredNamespaces, ns)
		}
	}

	ignoredServices := Services{
		{
			Name:      "kubernetes",
			Namespace: metav1.NamespaceDefault,
		},
	}

	return IgnoreWrapper{
		Namespaces:    ignoredNamespaces,
		Services:      ignoredServices,
		MeshNamespace: meshNamespace,
	}
}
