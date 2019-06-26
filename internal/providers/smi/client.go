package smi

import (
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/containous/traefik/pkg/log"
	"github.com/containous/traefik/pkg/provider/kubernetes/crd/generated/clientset/versioned"
	"github.com/containous/traefik/pkg/provider/kubernetes/crd/generated/informers/externalversions"
	"github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiAccessClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	smiAccessExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	smiSpecsClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	smiSpecsExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const resyncPeriod = 10 * time.Minute

type resourceEventHandler struct {
	ev chan<- interface{}
}

func (reh *resourceEventHandler) OnAdd(obj interface{}) {
	eventHandlerFunc(reh.ev, obj)
}

func (reh *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	eventHandlerFunc(reh.ev, newObj)
}

func (reh *resourceEventHandler) OnDelete(obj interface{}) {
	eventHandlerFunc(reh.ev, obj)
}

// Client is a client for the Provider master.
// WatchAll starts the watch of the Provider resources and updates the stores.
// The stores can then be accessed via the Get* functions.
type Client interface {
	WatchAll(namespaces []string, stopCh <-chan struct{}) (<-chan interface{}, error)

	GetMiddlewares() []*v1alpha1.Middleware

	GetNamespaces() []*corev1.Namespace
	GetPod(namespace, name string) (*corev1.Pod, bool, error)
	GetService(namespace, name string) (*corev1.Service, bool, error)
	GetServicesInNamespace(namespace string) []*corev1.Service
	GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error)

	GetTrafficTargets() []*accessv1alpha1.TrafficTarget
	GetTrafficTargetsWithDestinationInNamespace(namespace string) []*accessv1alpha1.TrafficTarget

	GetHTTPRouteGroup(namespace, name string) (*specsv1alpha1.HTTPRouteGroup, bool, error)
}

// TODO: add tests for the clientWrapper (and its methods) itself.
type clientWrapper struct {
	csCrd    *versioned.Clientset
	csKube   *kubernetes.Clientset
	csAccess *smiAccessClientset.Clientset
	csSpecs  *smiSpecsClientset.Clientset

	factoriesCrd    map[string]externalversions.SharedInformerFactory
	factoriesKube   map[string]informers.SharedInformerFactory
	factoriesAccess map[string]smiAccessExternalversions.SharedInformerFactory
	factoriesSpecs  map[string]smiSpecsExternalversions.SharedInformerFactory

	labelSelector labels.Selector

	isNamespaceAll    bool
	watchedNamespaces []string
}

func createClientFromConfig(c *rest.Config) (*clientWrapper, error) {
	csCrd, err := versioned.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	csKube, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	csAccess, err := smiAccessClientset.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	csSpecs, err := smiSpecsClientset.NewForConfig(c)
	if err != nil {
		return nil, err
	}

	return newClientImpl(csKube, csCrd, csAccess, csSpecs), nil
}

func newClientImpl(csKube *kubernetes.Clientset, csCrd *versioned.Clientset, csAccess *smiAccessClientset.Clientset, csSpecs *smiSpecsClientset.Clientset) *clientWrapper {
	return &clientWrapper{
		csCrd:           csCrd,
		csKube:          csKube,
		csAccess:        csAccess,
		csSpecs:         csSpecs,
		factoriesCrd:    make(map[string]externalversions.SharedInformerFactory),
		factoriesKube:   make(map[string]informers.SharedInformerFactory),
		factoriesAccess: make(map[string]smiAccessExternalversions.SharedInformerFactory),
		factoriesSpecs:  make(map[string]smiSpecsExternalversions.SharedInformerFactory),
	}
}

// newInClusterClient returns a new Provider client that is expected to run
// inside the cluster.
func newInClusterClient(endpoint string) (*clientWrapper, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster configuration: %s", err)
	}

	if endpoint != "" {
		config.Host = endpoint
	}

	return createClientFromConfig(config)
}

func newExternalClusterClientFromFile(file string) (*clientWrapper, error) {
	configFromFlags, err := clientcmd.BuildConfigFromFlags("", file)
	if err != nil {
		return nil, err
	}
	return createClientFromConfig(configFromFlags)
}

// newExternalClusterClient returns a new Provider client that may run outside
// of the cluster.
// The endpoint parameter must not be empty.
func newExternalClusterClient(endpoint, token, caFilePath string) (*clientWrapper, error) {
	if endpoint == "" {
		return nil, errors.New("endpoint missing for external cluster client")
	}

	config := &rest.Config{
		Host:        endpoint,
		BearerToken: token,
	}

	if caFilePath != "" {
		caData, err := ioutil.ReadFile(caFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file %s: %s", caFilePath, err)
		}

		config.TLSClientConfig = rest.TLSClientConfig{CAData: caData}
	}

	return createClientFromConfig(config)
}

// WatchAll starts namespace-specific controllers for all relevant kinds.
func (c *clientWrapper) WatchAll(namespaces []string, stopCh <-chan struct{}) (<-chan interface{}, error) {
	eventCh := make(chan interface{}, 1)
	eventHandler := c.newResourceEventHandler(eventCh)

	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
		c.isNamespaceAll = true
	}
	c.watchedNamespaces = namespaces

	for _, ns := range namespaces {
		factoryCrd := externalversions.NewSharedInformerFactoryWithOptions(c.csCrd, resyncPeriod, externalversions.WithNamespace(ns))
		factoryCrd.Traefik().V1alpha1().Middlewares().Informer().AddEventHandler(eventHandler)

		factoryKube := informers.NewFilteredSharedInformerFactory(c.csKube, resyncPeriod, ns, nil)
		factoryKube.Core().V1().Services().Informer().AddEventHandler(eventHandler)
		factoryKube.Core().V1().Endpoints().Informer().AddEventHandler(eventHandler)
		factoryKube.Core().V1().Pods().Informer().AddEventHandler(eventHandler)
		factoryKube.Core().V1().Namespaces().Informer().AddEventHandler(eventHandler)

		factoryAccess := smiAccessExternalversions.NewSharedInformerFactoryWithOptions(c.csAccess, resyncPeriod, smiAccessExternalversions.WithNamespace(ns))
		factoryAccess.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(eventHandler)

		factorySpecs := smiSpecsExternalversions.NewSharedInformerFactoryWithOptions(c.csSpecs, resyncPeriod, smiSpecsExternalversions.WithNamespace(ns))
		factorySpecs.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(eventHandler)

		c.factoriesCrd[ns] = factoryCrd
		c.factoriesKube[ns] = factoryKube
		c.factoriesAccess[ns] = factoryAccess
		c.factoriesSpecs[ns] = factorySpecs
	}

	for _, ns := range namespaces {
		c.factoriesCrd[ns].Start(stopCh)
		c.factoriesKube[ns].Start(stopCh)
		c.factoriesAccess[ns].Start(stopCh)
		c.factoriesSpecs[ns].Start(stopCh)
	}

	for _, ns := range namespaces {
		for t, ok := range c.factoriesCrd[ns].WaitForCacheSync(stopCh) {
			if !ok {
				return nil, fmt.Errorf("timed out waiting for controller caches to sync %s in namespace %q", t.String(), ns)
			}
		}

		for t, ok := range c.factoriesKube[ns].WaitForCacheSync(stopCh) {
			if !ok {
				return nil, fmt.Errorf("timed out waiting for controller caches to sync %s in namespace %q", t.String(), ns)
			}
		}
		for t, ok := range c.factoriesAccess[ns].WaitForCacheSync(stopCh) {
			if !ok {
				return nil, fmt.Errorf("timed out waiting for controller caches to sync %s in namespace %q", t.String(), ns)
			}
		}
		for t, ok := range c.factoriesSpecs[ns].WaitForCacheSync(stopCh) {
			if !ok {
				return nil, fmt.Errorf("timed out waiting for controller caches to sync %s in namespace %q", t.String(), ns)
			}
		}
	}

	return eventCh, nil
}

func (c *clientWrapper) GetMiddlewares() []*v1alpha1.Middleware {
	var result []*v1alpha1.Middleware

	for ns, factory := range c.factoriesCrd {
		ings, err := factory.Traefik().V1alpha1().Middlewares().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list ingresses in namespace %s: %s", ns, err)
		}
		result = append(result, ings...)
	}

	return result
}

// GetNamespaces returns all watched namespaces in the cluster.
func (c *clientWrapper) GetNamespaces() []*corev1.Namespace {
	var result []*corev1.Namespace
	for _, factory := range c.factoriesKube {
		namespaces, err := factory.Core().V1().Namespaces().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list namespaces: %v", err)
		}
		result = append(result, namespaces...)
	}

	return result
}

// GetService returns the named service from the given namespace.
func (c *clientWrapper) GetService(namespace, name string) (*corev1.Service, bool, error) {
	if !c.isWatchedNamespace(namespace) {
		return nil, false, fmt.Errorf("failed to get service %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	service, err := c.factoriesKube[c.lookupNamespace(namespace)].Core().V1().Services().Lister().Services(namespace).Get(name)
	exist, err := translateNotFoundError(err)
	return service, exist, err
}

// GetServicesInNamespace returns all services in the given namespace.
func (c *clientWrapper) GetServicesInNamespace(namespace string) []*corev1.Service {
	if !c.isWatchedNamespace(namespace) {
		log.Errorf("failed to get services: namespace %s is not within watched namespaces", namespace)
		return nil
	}

	services, err := c.factoriesKube[c.lookupNamespace(namespace)].Core().V1().Services().Lister().Services(namespace).List(c.labelSelector)
	if err != nil {
		log.Errorf("Failed to list services: %v", err)
	}
	return services
}

// GetEndpoints returns the named endpoints from the given namespace.
func (c *clientWrapper) GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error) {
	if !c.isWatchedNamespace(namespace) {
		return nil, false, fmt.Errorf("failed to get endpoints %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	endpoint, err := c.factoriesKube[c.lookupNamespace(namespace)].Core().V1().Endpoints().Lister().Endpoints(namespace).Get(name)
	exist, err := translateNotFoundError(err)
	return endpoint, exist, err
}

// GetPod returns the named od from the given namespace.
func (c *clientWrapper) GetPod(namespace, name string) (*corev1.Pod, bool, error) {
	if !c.isWatchedNamespace(namespace) {
		return nil, false, fmt.Errorf("failed to get pod %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	pod, err := c.factoriesKube[c.lookupNamespace(namespace)].Core().V1().Pods().Lister().Pods(namespace).Get(name)
	exist, err := translateNotFoundError(err)
	return pod, exist, err
}

// GetTrafficTargets returns all trafficTargets in watched namespaces in the cluster.
func (c *clientWrapper) GetTrafficTargets() []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget
	for _, factory := range c.factoriesAccess {
		trafficTargets, err := factory.Access().V1alpha1().TrafficTargets().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list TrafficTargets: %v", err)
		}
		result = append(result, trafficTargets...)
	}

	return result
}

// GetTrafficTargetsWithDestinationInNamespace returns trafficTargets with the destination SA in the given namespace.
func (c *clientWrapper) GetTrafficTargetsWithDestinationInNamespace(namespace string) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget
	allTrafficTargets := c.GetTrafficTargets()
	for _, trafficTarget := range allTrafficTargets {
		if trafficTarget.Destination.Namespace != namespace {
			continue
		}
		result = append(result, trafficTarget)
	}

	return result
}

// GetHTTPRouteGroup returns an HTTPRouteGroup with the given name and namespace.
func (c *clientWrapper) GetHTTPRouteGroup(namespace, name string) (*specsv1alpha1.HTTPRouteGroup, bool, error) {
	if !c.isWatchedNamespace(namespace) {
		return nil, false, fmt.Errorf("failed to get HTTPRouteGroup %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	hrg, err := c.factoriesSpecs[c.lookupNamespace(namespace)].Specs().V1alpha1().HTTPRouteGroups().Lister().HTTPRouteGroups(namespace).Get(name)
	exist, err := translateNotFoundError(err)
	return hrg, exist, err

}

// lookupNamespace returns the lookup namespace key for the given namespace.
// When listening on all namespaces, it returns the client-go identifier ("")
// for all-namespaces. Otherwise, it returns the given namespace.
// The distinction is necessary because we index all informers on the special
// identifier iff all-namespaces are requested but receive specific namespace
// identifiers from the Kubernetes API, so we have to bridge this gap.
func (c *clientWrapper) lookupNamespace(ns string) string {
	if c.isNamespaceAll {
		return metav1.NamespaceAll
	}
	return ns
}

func (c *clientWrapper) newResourceEventHandler(events chan<- interface{}) cache.ResourceEventHandler {
	return &cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			// Ignore Ingresses that do not match our custom label selector.
			if ing, ok := obj.(*extensionsv1beta1.Ingress); ok {
				lbls := labels.Set(ing.GetLabels())
				return c.labelSelector.Matches(lbls)
			}
			return true
		},
		Handler: &resourceEventHandler{ev: events},
	}
}

// eventHandlerFunc will pass the obj on to the events channel or drop it.
// This is so passing the events along won't block in the case of high volume.
// The events are only used for signaling anyway so dropping a few is ok.
func eventHandlerFunc(events chan<- interface{}, obj interface{}) {
	select {
	case events <- obj:
	default:
	}
}

// translateNotFoundError will translate a "not found" error to a boolean return
// value which indicates if the resource exists and a nil error.
func translateNotFoundError(err error) (bool, error) {
	if kubeerror.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// isWatchedNamespace checks to ensure that the namespace is being watched before we request
// it to ensure we don't panic by requesting an out-of-watch object.
func (c *clientWrapper) isWatchedNamespace(ns string) bool {
	if c.isNamespaceAll {
		return true
	}
	for _, watchedNamespace := range c.watchedNamespaces {
		if watchedNamespace == ns {
			return true
		}
	}
	return false
}
