package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

type ResourceFilter struct {
	watchedNamespaces []string
	ignoredNamespaces []string
	ignoredServices   []namespaceName
	ignoredApps       []string
}

type namespaceName struct {
	Name      string
	Namespace string
}

type ResourceFilterOption func(filter *ResourceFilter)

func WatchNamespaces(namespaces ...string) ResourceFilterOption {
	return func(filter *ResourceFilter) {
		filter.watchedNamespaces = append(filter.watchedNamespaces, namespaces...)
	}
}

func IgnoreNamespaces(namespaces ...string) ResourceFilterOption {
	return func(filter *ResourceFilter) {
		filter.ignoredNamespaces = append(filter.ignoredNamespaces, namespaces...)
	}
}

func IgnoreApps(apps ...string) ResourceFilterOption {
	return func(filter *ResourceFilter) {
		filter.ignoredApps = append(filter.ignoredApps, apps...)
	}
}

func IgnoreService(ns, name string) ResourceFilterOption {
	return func(filter *ResourceFilter) {
		filter.ignoredServices = append(filter.ignoredServices, namespaceName{
			Namespace: ns,
			Name:      name,
		})
	}
}

func NewResourceFilter(opts ...ResourceFilterOption) *ResourceFilter {
	var filter ResourceFilter

	for _, opt := range opts {
		opt(&filter)
	}

	return &filter
}

func (f *ResourceFilter) IsIgnored(obj interface{}) bool {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return true
	}

	pMeta := meta.AsPartialObjectMetadata(accessor)

	// If we are not watching all namespaces, check if the namespace is in the watch list.
	if len(f.watchedNamespaces) > 0 && !contains(f.watchedNamespaces, pMeta.Namespace) {
		return true
	}

	// Check if the namespace is not explicitly ignored.
	if contains(f.ignoredNamespaces, pMeta.Namespace) {
		return true
	}

	// Check if the "app" label doesn't contain a value which is ignored.
	if contains(f.ignoredApps, pMeta.Labels["app"]) {
		return true
	}

	if svc, ok := obj.(*corev1.Service); ok {
		// Check if the service is nt explicitly ignored.
		if containsNamespaceName(f.ignoredServices, namespaceName{Namespace: svc.Namespace, Name: svc.Name}) {
			return true
		}

		// Ignore ExternalName services as they are not currently supported.
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			return true
		}
	}

	return false
}

func contains(sources []string, target string) bool {
	for _, source := range sources {
		if source == target {
			return true
		}
	}

	return false
}

func containsNamespaceName(sources []namespaceName, target namespaceName) bool {
	for _, source := range sources {
		if source.Namespace == target.Namespace && source.Name == target.Name {
			return true
		}
	}

	return false
}
