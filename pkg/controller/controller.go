package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/containous/maesh/pkg/annotations"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/provider"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	accesslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	specslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	splitlister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// configRefreshKey is the work queue key used to indicate that config has to be refreshed.
	configRefreshKey = "refresh"

	// maxRetries is the number of times a work task will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times a
	// work task is going to be re-queued: 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s
	maxRetries = 12
)

// SharedStore is used to share the controller state.
type SharedStore interface {
	SetConfig(cfg *dynamic.Configuration)
	SetTopology(topo *topology.Topology)
	SetReadiness(isReady bool)
}

// TopologyBuilder builds Topologies.
type TopologyBuilder interface {
	Build(resourceFilter *k8s.ResourceFilter) (*topology.Topology, error)
}

// Config holds the configuration of the controller.
type Config struct {
	ACLEnabled       bool
	DefaultMode      string
	Namespace        string
	WatchNamespaces  []string
	IgnoreNamespaces []string
	MinHTTPPort      int32
	MaxHTTPPort      int32
	MinTCPPort       int32
	MaxTCPPort       int32
	MinUDPPort       int32
	MaxUDPPort       int32
}

// Controller hold controller configuration.
type Controller struct {
	cfg                  Config
	workQueue            workqueue.RateLimitingInterface
	shadowServiceManager *ShadowServiceManager
	provider             *provider.Provider
	resourceFilter       *k8s.ResourceFilter
	tcpStateTable        *PortMapping
	udpStateTable        *PortMapping
	topologyBuilder      TopologyBuilder
	store                SharedStore
	logger               logrus.FieldLogger

	clients              k8s.Client
	kubernetesFactory    informers.SharedInformerFactory
	accessFactory        accessinformer.SharedInformerFactory
	specsFactory         specsinformer.SharedInformerFactory
	splitFactory         splitinformer.SharedInformerFactory
	podLister            listers.PodLister
	serviceLister        listers.ServiceLister
	endpointsLister      listers.EndpointsLister
	trafficTargetLister  accesslister.TrafficTargetLister
	httpRouteGroupLister specslister.HTTPRouteGroupLister
	tcpRouteLister       specslister.TCPRouteLister
	trafficSplitLister   splitlister.TrafficSplitLister
}

// NewMeshController builds the informers and other required components of the mesh controller, and returns an
// initialized mesh controller object.
func NewMeshController(clients k8s.Client, cfg Config, store SharedStore, logger logrus.FieldLogger) *Controller {
	c := &Controller{
		logger:  logger,
		cfg:     cfg,
		clients: clients,
		store:   store,
	}

	// Initialize the ignored and watched resources.
	c.resourceFilter = k8s.NewResourceFilter(
		k8s.WatchNamespaces(cfg.WatchNamespaces...),
		k8s.IgnoreNamespaces(cfg.IgnoreNamespaces...),
		k8s.IgnoreNamespaces(metav1.NamespaceSystem),
		k8s.IgnoreService(metav1.NamespaceDefault, "kubernetes"),
		k8s.IgnoreApps("maesh", "jaeger"),
	)

	// Create the work queue and the enqueue handler.
	c.workQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	handler := cache.FilteringResourceEventHandler{
		FilterFunc: c.isWatchedResource,
		Handler:    &enqueueWorkHandler{logger: c.logger, workQueue: c.workQueue},
	}

	// Create SharedInformers, listers and register the event handler to informers that are not ACL related.
	c.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.KubernetesClient(), k8s.ResyncPeriod)
	c.splitFactory = splitinformer.NewSharedInformerFactoryWithOptions(c.clients.SplitClient(), k8s.ResyncPeriod)

	c.podLister = c.kubernetesFactory.Core().V1().Pods().Lister()
	c.endpointsLister = c.kubernetesFactory.Core().V1().Endpoints().Lister()
	c.serviceLister = c.kubernetesFactory.Core().V1().Services().Lister()
	c.trafficSplitLister = c.splitFactory.Split().V1alpha2().TrafficSplits().Lister()

	c.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(handler)
	c.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(handler)
	c.splitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(handler)

	// Create SharedInformers, listers and register the event handler for ACL related resources.
	if c.cfg.ACLEnabled {
		c.accessFactory = accessinformer.NewSharedInformerFactoryWithOptions(c.clients.AccessClient(), k8s.ResyncPeriod)
		c.specsFactory = specsinformer.NewSharedInformerFactoryWithOptions(c.clients.SpecsClient(), k8s.ResyncPeriod)

		c.trafficTargetLister = c.accessFactory.Access().V1alpha1().TrafficTargets().Lister()
		c.httpRouteGroupLister = c.specsFactory.Specs().V1alpha1().HTTPRouteGroups().Lister()
		c.tcpRouteLister = c.specsFactory.Specs().V1alpha1().TCPRoutes().Lister()

		c.accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(handler)
		c.kubernetesFactory.Core().V1().Pods().Informer().AddEventHandler(handler)
		c.specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(handler)
		c.specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(handler)
	}

	c.tcpStateTable = NewPortMapping(c.cfg.Namespace, c.serviceLister, logger, c.cfg.MinTCPPort, c.cfg.MaxTCPPort)

	c.udpStateTable = NewPortMapping(c.cfg.Namespace, c.serviceLister, logger, c.cfg.MinUDPPort, c.cfg.MaxUDPPort)

	c.shadowServiceManager = NewShadowServiceManager(
		c.logger,
		c.serviceLister,
		c.cfg.Namespace,
		c.tcpStateTable,
		c.udpStateTable,
		c.cfg.DefaultMode,
		c.cfg.MinHTTPPort,
		c.cfg.MaxHTTPPort,
		c.clients.KubernetesClient(),
	)

	c.topologyBuilder = topology.NewBuilder(
		c.serviceLister,
		c.endpointsLister,
		c.podLister,
		c.trafficTargetLister,
		c.trafficSplitLister,
		c.httpRouteGroupLister,
		c.tcpRouteLister,
		c.logger,
	)

	providerCfg := provider.Config{
		MinHTTPPort:        c.cfg.MinHTTPPort,
		MaxHTTPPort:        c.cfg.MaxHTTPPort,
		ACL:                c.cfg.ACLEnabled,
		DefaultTrafficType: c.cfg.DefaultMode,
	}

	c.provider = provider.New(c.tcpStateTable, c.udpStateTable, annotations.BuildMiddlewares, providerCfg, c.logger)

	return c
}

// Run is the main entrypoint for the controller.
func (c *Controller) Run(stopCh <-chan struct{}) error {
	// Handle a panic with logging and exiting.
	defer utilruntime.HandleCrash()

	waitGroup := sync.WaitGroup{}

	defer func() {
		c.logger.Info("Shutting down workers")
		c.workQueue.ShutDown()

		waitGroup.Wait()
	}()

	c.logger.Debug("Initializing mesh controller")

	// Start the informers.
	if err := c.startInformers(stopCh, 10*time.Second); err != nil {
		return fmt.Errorf("could not start informers: %w", err)
	}

	// Load the TCP and UDP port mapper states.
	if err := c.loadPortMappersState(); err != nil {
		return fmt.Errorf("could not load port mapper states: %w", err)
	}

	// Enable API readiness endpoint, informers are started and default conf is available.
	c.store.SetReadiness(true)

	// Start to poll work from the queue.
	waitGroup.Add(1)

	runWorker := func() {
		defer waitGroup.Done()
		c.runWorker()
	}

	go wait.Until(runWorker, time.Second, stopCh)

	<-stopCh

	return nil
}

// startInformers starts the controller informers.
func (c *Controller) startInformers(stopCh <-chan struct{}, syncTimeout time.Duration) error {
	// Start the informers with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	c.logger.Debug("Starting Informers")

	if err := c.startBaseInformers(ctx, stopCh); err != nil {
		return err
	}

	if c.cfg.ACLEnabled {
		if err := c.startACLInformers(ctx, stopCh); err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) startBaseInformers(ctx context.Context, stopCh <-chan struct{}) error {
	c.kubernetesFactory.Start(stopCh)

	for t, ok := range c.kubernetesFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t)
		}
	}

	c.splitFactory.Start(stopCh)

	for t, ok := range c.splitFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t)
		}
	}

	return nil
}

func (c *Controller) startACLInformers(ctx context.Context, stopCh <-chan struct{}) error {
	c.accessFactory.Start(stopCh)

	for t, ok := range c.accessFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t)
		}
	}

	c.specsFactory.Start(stopCh)

	for t, ok := range c.specsFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t)
		}
	}

	return nil
}

// loadPortMappersState loads the TCP and UDP port mapper states.
func (c *Controller) loadPortMappersState() error {
	if err := c.tcpStateTable.LoadState(); err != nil {
		return fmt.Errorf("unable to load TCP state table: %w", err)
	}

	if err := c.udpStateTable.LoadState(); err != nil {
		return fmt.Errorf("unable to load UDP state table: %w", err)
	}

	return nil
}

// isWatchedResource returns true if the given resource is not ignored, false otherwise.
func (c *Controller) isWatchedResource(obj interface{}) bool {
	return !c.resourceFilter.IsIgnored(obj)
}

// runWorker is a long-running function that will continually call the processNextWorkItem function in order to read and
// process a message on the work queue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the work queue and attempt to process it.
func (c *Controller) processNextWorkItem() bool {
	key, quit := c.workQueue.Get()
	if quit {
		return false
	}

	defer c.workQueue.Done(key)

	if key != configRefreshKey {
		if err := c.syncShadowService(key.(string)); err != nil {
			c.handleErr(key, fmt.Errorf("unable to sync shadow service: %w", err))
			return true
		}
	}

	// Build and store config.
	topo, err := c.topologyBuilder.Build(c.resourceFilter)
	if err != nil {
		c.handleErr(key, fmt.Errorf("unable to build topology: %w", err))
		return true
	}

	conf := c.provider.BuildConfig(topo)

	c.store.SetTopology(topo)
	c.store.SetConfig(conf)

	c.workQueue.Forget(key)

	return true
}

// syncShadowService calls the shadow service manager to keep the shadow service state in sync with the service events received.
func (c *Controller) syncShadowService(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	svc, err := c.serviceLister.Services(namespace).Get(name)
	if errors.IsNotFound(err) {
		return c.shadowServiceManager.Delete(namespace, name)
	}

	if err != nil {
		return err
	}

	_, err = c.shadowServiceManager.CreateOrUpdate(svc)
	if err != nil {
		return err
	}

	return nil
}

// handleErr re-queues the given work key only if the maximum number of attempts is not exceeded.
func (c *Controller) handleErr(key interface{}, err error) {
	if c.workQueue.NumRequeues(key) < maxRetries {
		c.workQueue.AddRateLimited(key)
		return
	}

	c.logger.Errorf("Unable to complete work %q: %v", key, err)
	c.workQueue.Forget(key)
}
