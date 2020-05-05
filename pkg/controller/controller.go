package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/containous/maesh/pkg/annotations"
	"github.com/containous/maesh/pkg/api"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/provider"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/safe"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	accesslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	specslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	splitlister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
)

// PortMapper is capable of storing and retrieving a port mapping for a given service.
type PortMapper interface {
	Find(svc k8s.ServiceWithPort) (int32, bool)
	Add(svc *k8s.ServiceWithPort) (int32, error)
	Remove(svc k8s.ServiceWithPort) (int32, error)
}

// TopologyBuilder builds Topologies.
type TopologyBuilder interface {
	Build(ignoredResources k8s.IgnoreWrapper) (*topology.Topology, error)
}

// ServiceManager is capable of managing kubernetes services.
type ServiceManager interface {
	Create(userSvc *corev1.Service) error
	Update(oldUserSvc, newUserSvc *corev1.Service) (*corev1.Service, error)
	Delete(userSvc *corev1.Service) error
}

// Config holds the configuration of the controller.
type Config struct {
	ACLEnabled       bool
	DefaultMode      string
	Namespace        string
	IgnoreNamespaces []string
	APIPort          int32
	APIHost          string
	MinHTTPPort      int32
	MaxHTTPPort      int32
	MinTCPPort       int32
	MaxTCPPort       int32
	MinUDPPort       int32
	MaxUDPPort       int32
}

// Controller hold controller configuration.
type Controller struct {
	cfg               Config
	handler           *Handler
	serviceManager    ServiceManager
	configRefreshChan chan string
	provider          *provider.Provider
	ignoredResources  k8s.IgnoreWrapper
	tcpStateTable     PortMapper
	udpStateTable     PortMapper
	topologyBuilder   TopologyBuilder
	lastConfiguration safe.Safe
	api               api.Interface
	logger            logrus.FieldLogger

	clients              k8s.Client
	kubernetesFactory    informers.SharedInformerFactory
	accessFactory        accessinformer.SharedInformerFactory
	specsFactory         specsinformer.SharedInformerFactory
	splitFactory         splitinformer.SharedInformerFactory
	PodLister            listers.PodLister
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	TrafficTargetLister  accesslister.TrafficTargetLister
	HTTPRouteGroupLister specslister.HTTPRouteGroupLister
	TCPRouteLister       specslister.TCPRouteLister
	TrafficSplitLister   splitlister.TrafficSplitLister
}

// NewMeshController is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients k8s.Client, cfg Config, logger logrus.FieldLogger) (*Controller, error) {
	ignoredResources := k8s.NewIgnored()

	for _, ns := range cfg.IgnoreNamespaces {
		ignoredResources.AddIgnoredNamespace(ns)
	}

	ignoredResources.AddIgnoredService("kubernetes", metav1.NamespaceDefault)
	ignoredResources.AddIgnoredNamespace(metav1.NamespaceSystem)
	ignoredResources.AddIgnoredApps("maesh", "jaeger")

	tcpStateTable, err := k8s.NewPortMapping(clients.GetKubernetesClient(), cfg.Namespace, k8s.TCPStateConfigMapName, cfg.MinTCPPort, cfg.MaxTCPPort)
	if err != nil {
		return nil, err
	}

	udpStateTable, err := k8s.NewPortMapping(clients.GetKubernetesClient(), cfg.Namespace, k8s.UDPStateConfigMapName, cfg.MinUDPPort, cfg.MaxUDPPort)
	if err != nil {
		return nil, err
	}

	c := &Controller{
		logger:           logger,
		cfg:              cfg,
		clients:          clients,
		ignoredResources: ignoredResources,
		tcpStateTable:    tcpStateTable,
		udpStateTable:    udpStateTable,
	}

	c.init()

	return c, nil
}

func (c *Controller) init() {
	// Create SharedInformers for non-ACL related resources.
	c.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.GetKubernetesClient(), k8s.ResyncPeriod)
	c.splitFactory = splitinformer.NewSharedInformerFactoryWithOptions(c.clients.GetSplitClient(), k8s.ResyncPeriod)

	c.ServiceLister = c.kubernetesFactory.Core().V1().Services().Lister()
	c.serviceManager = NewShadowServiceManager(c.logger, c.ServiceLister, c.cfg.Namespace, c.tcpStateTable, c.udpStateTable, c.cfg.DefaultMode, c.cfg.MinHTTPPort, c.cfg.MaxHTTPPort, c.clients.GetKubernetesClient())

	// configRefreshChan is used to trigger configuration refreshes and deploys.
	c.configRefreshChan = make(chan string)
	c.handler = NewHandler(c.logger, c.ignoredResources, c.serviceManager, c.configRefreshChan)

	// Create listers and register the event handler to informers that are not ACL related.
	c.PodLister = c.kubernetesFactory.Core().V1().Pods().Lister()
	c.EndpointsLister = c.kubernetesFactory.Core().V1().Endpoints().Lister()
	c.TrafficSplitLister = c.splitFactory.Split().V1alpha2().TrafficSplits().Lister()

	c.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Pods().Informer().AddEventHandler(c.handler)
	c.splitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(c.handler)

	// Create SharedInformers, listers and register the event handler for ACL related resources.
	if c.cfg.ACLEnabled {
		c.accessFactory = accessinformer.NewSharedInformerFactoryWithOptions(c.clients.GetAccessClient(), k8s.ResyncPeriod)
		c.specsFactory = specsinformer.NewSharedInformerFactoryWithOptions(c.clients.GetSpecsClient(), k8s.ResyncPeriod)

		c.TrafficTargetLister = c.accessFactory.Access().V1alpha1().TrafficTargets().Lister()
		c.HTTPRouteGroupLister = c.specsFactory.Specs().V1alpha1().HTTPRouteGroups().Lister()
		c.TCPRouteLister = c.specsFactory.Specs().V1alpha1().TCPRoutes().Lister()

		c.accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(c.handler)
		c.specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(c.handler)
		c.specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(c.handler)
	}

	c.api = api.NewAPI(c.logger, c.cfg.APIPort, c.cfg.APIHost, &c.lastConfiguration, c.PodLister, c.cfg.Namespace)

	c.topologyBuilder = &topology.Builder{
		ServiceLister:        c.ServiceLister,
		EndpointsLister:      c.EndpointsLister,
		PodLister:            c.PodLister,
		TrafficTargetLister:  c.TrafficTargetLister,
		TrafficSplitLister:   c.TrafficSplitLister,
		HTTPRouteGroupLister: c.HTTPRouteGroupLister,
		TCPRoutesLister:      c.TCPRouteLister,
		Logger:               c.logger,
	}

	providerCfg := provider.Config{
		IgnoredResources:   c.ignoredResources,
		MinHTTPPort:        c.cfg.MinHTTPPort,
		MaxHTTPPort:        c.cfg.MaxHTTPPort,
		ACL:                c.cfg.ACLEnabled,
		DefaultTrafficType: c.cfg.DefaultMode,
	}

	c.provider = provider.New(c.tcpStateTable, c.udpStateTable, annotations.BuildMiddlewares, providerCfg, c.logger)
}

// Run is the main entrypoint for the controller.
func (c *Controller) Run(stopCh <-chan struct{}) error {
	// Handle a panic with logging and exiting.
	defer utilruntime.HandleCrash()

	c.logger.Debug("Initializing Mesh controller")

	// Start the informers.
	c.startInformers(stopCh, 10*time.Second)

	// Create the mesh services here to ensure that they exist.
	c.logger.Info("Creating initial mesh services")

	if err := c.createMeshServices(); err != nil {
		c.logger.Errorf("could not create mesh services: %v", err)
	}

	// Start the api, and enable the readiness endpoint.
	c.api.Start()

	for {
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-stopCh:
			c.logger.Info("Shutting down workers")
			return nil
		case message := <-c.configRefreshChan:
			// Reload the configuration.
			topo, err := c.topologyBuilder.Build(c.ignoredResources)
			if err != nil {
				c.logger.Errorf("Unable to build dynamic configuration: %v", err)
				continue
			}

			conf := c.provider.BuildConfig(topo)

			if message == k8s.ConfigMessageChanForce || !reflect.DeepEqual(c.lastConfiguration.Get(), conf) {
				c.lastConfiguration.Set(conf)

				// Configuration successfully created, enable readiness in the api.
				c.api.EnableReadiness()
			}
		case <-timer.C:
			rawCfg := c.lastConfiguration.Get()
			if rawCfg == nil {
				break
			}

			// Configuration successfully created, enable readiness in the api.
			c.api.EnableReadiness()
		}
	}
}

// startInformers starts the controller informers.
func (c *Controller) startInformers(stopCh <-chan struct{}, syncTimeout time.Duration) {
	// Start the informers with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	c.logger.Debug("Starting Informers")
	c.kubernetesFactory.Start(stopCh)

	for t, ok := range c.kubernetesFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			c.logger.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	c.splitFactory.Start(stopCh)

	for t, ok := range c.splitFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			c.logger.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if c.cfg.ACLEnabled {
		c.accessFactory.Start(stopCh)

		for t, ok := range c.accessFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				c.logger.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.specsFactory.Start(stopCh)

		for t, ok := range c.specsFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				c.logger.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}
}

func (c *Controller) createMeshServices() error {
	sel, err := c.ignoredResources.LabelSelector()
	if err != nil {
		return fmt.Errorf("unable to build label selectors: %w", err)
	}

	// Because createMeshServices is called after startInformers,
	// then we already have the cache built, so we can use it.
	svcs, err := c.ServiceLister.List(sel)
	if err != nil {
		return fmt.Errorf("unable to get services: %w", err)
	}

	for _, service := range svcs {
		if c.ignoredResources.IsIgnored(service.ObjectMeta) {
			continue
		}

		c.logger.Debugf("Creating mesh for service: %v", service.Name)

		if err := c.serviceManager.Create(service); err != nil {
			return fmt.Errorf("unable to create mesh service: %w", err)
		}
	}

	return nil
}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "maesh-mesh"
}
