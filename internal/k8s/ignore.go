package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IgnoreWrapper holds namespaces and services to ignore.
type IgnoreWrapper struct {
	Namespaces Namespaces
	Services   Services
}

func (i *IgnoreWrapper) Ignored(name, namespace string) bool {
	if i.Namespaces.Contains(namespace) {
		return true
	}

	if i.Services.Contains(name, namespace) {
		return true
	}
	return false
}

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
