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

	GetIngressRoutes() []*v1alpha1.IngressRoute
	GetIngressRouteTCPs() []*v1alpha1.IngressRouteTCP
	GetMiddlewares() []*v1alpha1.Middleware
	GetTLSOptions() []*v1alpha1.TLSOption

	GetIngresses() []*extensionsv1beta1.Ingress
	GetService(namespace, name string) (*corev1.Service, bool, error)
	GetSecret(namespace, name string) (*corev1.Secret, bool, error)
	GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error)
	UpdateIngressStatus(namespace, name, ip, hostname string) error
}

// TODO: add tests for the clientWrapper (and its methods) itself.
type clientWrapper struct {
	csCrd  *versioned.Clientset
	csKube *kubernetes.Clientset

	factoriesCrd  map[string]externalversions.SharedInformerFactory
	factoriesKube map[string]informers.SharedInformerFactory

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

	return newClientImpl(csKube, csCrd), nil
}

func newClientImpl(csKube *kubernetes.Clientset, csCrd *versioned.Clientset) *clientWrapper {
	return &clientWrapper{
		csCrd:         csCrd,
		csKube:        csKube,
		factoriesCrd:  make(map[string]externalversions.SharedInformerFactory),
		factoriesKube: make(map[string]informers.SharedInformerFactory),
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
		factoryCrd.Traefik().V1alpha1().IngressRoutes().Informer().AddEventHandler(eventHandler)
		factoryCrd.Traefik().V1alpha1().Middlewares().Informer().AddEventHandler(eventHandler)
		factoryCrd.Traefik().V1alpha1().IngressRouteTCPs().Informer().AddEventHandler(eventHandler)
		factoryCrd.Traefik().V1alpha1().TLSOptions().Informer().AddEventHandler(eventHandler)

		factoryKube := informers.NewFilteredSharedInformerFactory(c.csKube, resyncPeriod, ns, nil)
		factoryKube.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(eventHandler)
		factoryKube.Core().V1().Services().Informer().AddEventHandler(eventHandler)
		factoryKube.Core().V1().Endpoints().Informer().AddEventHandler(eventHandler)

		c.factoriesCrd[ns] = factoryCrd
		c.factoriesKube[ns] = factoryKube
	}

	for _, ns := range namespaces {
		c.factoriesCrd[ns].Start(stopCh)
		c.factoriesKube[ns].Start(stopCh)
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
	}

	// Do not wait for the Secrets store to get synced since we cannot rely on
	// users having granted RBAC permissions for this object.
	// https://github.com/containous/traefik/issues/1784 should improve the
	// situation here in the future.
	for _, ns := range namespaces {
		c.factoriesKube[ns].Core().V1().Secrets().Informer().AddEventHandler(eventHandler)
		c.factoriesKube[ns].Start(stopCh)
	}

	return eventCh, nil
}

func (c *clientWrapper) GetIngressRoutes() []*v1alpha1.IngressRoute {
	var result []*v1alpha1.IngressRoute

	for ns, factory := range c.factoriesCrd {
		ings, err := factory.Traefik().V1alpha1().IngressRoutes().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list ingresses in namespace %s: %s", ns, err)
		}
		result = append(result, ings...)
	}

	return result
}

func (c *clientWrapper) GetIngressRouteTCPs() []*v1alpha1.IngressRouteTCP {
	var result []*v1alpha1.IngressRouteTCP

	for ns, factory := range c.factoriesCrd {
		ings, err := factory.Traefik().V1alpha1().IngressRouteTCPs().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list tcp ingresses in namespace %s: %s", ns, err)
		}
		result = append(result, ings...)
	}

	return result
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

// GetTLSOptions
func (c *clientWrapper) GetTLSOptions() []*v1alpha1.TLSOption {
	var result []*v1alpha1.TLSOption

	for ns, factory := range c.factoriesCrd {
		options, err := factory.Traefik().V1alpha1().TLSOptions().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list tls options in namespace %s: %s", ns, err)
		}
		result = append(result, options...)
	}

	return result
}

// GetIngresses returns all Ingresses for observed namespaces in the cluster.
func (c *clientWrapper) GetIngresses() []*extensionsv1beta1.Ingress {
	var result []*extensionsv1beta1.Ingress
	for ns, factory := range c.factoriesKube {
		ings, err := factory.Extensions().V1beta1().Ingresses().Lister().List(c.labelSelector)
		if err != nil {
			log.Errorf("Failed to list ingresses in namespace %s: %s", ns, err)
		}
		result = append(result, ings...)
	}
	return result
}

// UpdateIngressStatus updates an Ingress with a provided status.
func (c *clientWrapper) UpdateIngressStatus(namespace, name, ip, hostname string) error {
	if !c.isWatchedNamespace(namespace) {
		return fmt.Errorf("failed to get ingress %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	ing, err := c.factoriesKube[c.lookupNamespace(namespace)].Extensions().V1beta1().Ingresses().Lister().Ingresses(namespace).Get(name)
	if err != nil {
		return fmt.Errorf("failed to get ingress %s/%s: %v", namespace, name, err)
	}

	if len(ing.Status.LoadBalancer.Ingress) > 0 {
		if ing.Status.LoadBalancer.Ingress[0].Hostname == hostname && ing.Status.LoadBalancer.Ingress[0].IP == ip {
			// If status is already set, skip update
			log.Debugf("Skipping status update on ingress %s/%s", ing.Namespace, ing.Name)
			return nil
		}
	}
	ingCopy := ing.DeepCopy()
	ingCopy.Status = extensionsv1beta1.IngressStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: ip, Hostname: hostname}}}}

	_, err = c.csKube.ExtensionsV1beta1().Ingresses(ingCopy.Namespace).UpdateStatus(ingCopy)
	if err != nil {
		return fmt.Errorf("failed to update ingress status %s/%s: %v", namespace, name, err)
	}
	log.Infof("Updated status on ingress %s/%s", namespace, name)
	return nil
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

// GetEndpoints returns the named endpoints from the given namespace.
func (c *clientWrapper) GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error) {
	if !c.isWatchedNamespace(namespace) {
		return nil, false, fmt.Errorf("failed to get endpoints %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	endpoint, err := c.factoriesKube[c.lookupNamespace(namespace)].Core().V1().Endpoints().Lister().Endpoints(namespace).Get(name)
	exist, err := translateNotFoundError(err)
	return endpoint, exist, err
}

// GetSecret returns the named secret from the given namespace.
func (c *clientWrapper) GetSecret(namespace, name string) (*corev1.Secret, bool, error) {
	if !c.isWatchedNamespace(namespace) {
		return nil, false, fmt.Errorf("failed to get secret %s/%s: namespace is not within watched namespaces", namespace, name)
	}

	secret, err := c.factoriesKube[c.lookupNamespace(namespace)].Core().V1().Secrets().Lister().Secrets(namespace).Get(name)
	exist, err := translateNotFoundError(err)
	return secret, exist, err
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
