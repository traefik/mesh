package mesh

import (
	"github.com/containous/i3o/internal/k8s"
	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiSplitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func newServiceListWatch(client k8s.CoreV1Client) (string, *cache.ListWatch, runtime.Object) {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			// list all of the services (core resource) in all namespaces
			return client.ListServicesWithOptions(metav1.NamespaceAll, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			// watch all of the services (core resource) in all namespaces
			return client.WatchServicesWithOptions(metav1.NamespaceAll, options)
		},
	}

	return "service", lw, &corev1.Service{}
}

func newTrafficTargetListWatch(client k8s.SMIAccessV1Alpha1Client) (string, *cache.ListWatch, runtime.Object) {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			// list all of the traffic targets (SMI access object) in all namespaces
			return client.ListTrafficTargetsWithOptions(metav1.NamespaceAll, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			// watch all of the traffic targets (SMI access object) in all namespaces
			return client.WatchTrafficTargetsWithOptions(metav1.NamespaceAll, options)
		},
	}

	return "traffictarget", lw, &smiAccessv1alpha1.TrafficTarget{}
}

func newHTTPRouteGroupListWatch(client k8s.SMISpecsV1Alpha1Client) (string, *cache.ListWatch, runtime.Object) {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			// list all of the httproutegroups (SMI specs object) in all namespaces
			return client.ListHTTPRouteGroupsWithOptions(metav1.NamespaceAll, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			// watch all of the httproutegroups (SMI specs object) in all namespaces
			return client.WatchHTTPRouteGroupsWithOptions(metav1.NamespaceAll, options)
		},
	}

	return "httproutegroup", lw, &smiSpecsv1alpha1.HTTPRouteGroup{}
}

func newTrafficSplitListWatch(client k8s.SMISplitV1Alpha1Client) (string, *cache.ListWatch, runtime.Object) {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			// list all of the traffic splits (SMI specs object) in all namespaces
			return client.ListTrafficSplitsWithOptions(metav1.NamespaceAll, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			// watch all of the traffic splits (SMI specs object) in all namespaces
			return client.WatchTrafficSplitsWithOptions(metav1.NamespaceAll, options)
		},
	}

	return "trafficsplit", lw, &smiSplitv1alpha1.TrafficSplit{}
}
