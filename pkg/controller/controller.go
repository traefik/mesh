package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/maesh/pkg/api"
	"github.com/containous/maesh/pkg/deploylog"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/providers/base"
	"github.com/containous/maesh/pkg/providers/kubernetes"
	"github.com/containous/maesh/pkg/providers/smi"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	accessInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	accessLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specsInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	specsLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	splitLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
)

// TCPPortMapper is capable of storing and retrieving a TCP port mapping for a given service.
type TCPPortMapper interface {
	Find(svc k8s.ServiceWithPort) (int32, bool)
	Add(svc *k8s.ServiceWithPort) (int32, error)
	Remove(svc k8s.ServiceWithPort) (int32, error)
}

// ServiceManager is capable of managing kubernetes services.
type ServiceManager interface {
	Create(userSvc *corev1.Service) error
	Update(oldUserSvc, newUserSvc *corev1.Service) (*corev1.Service, error)
	Delete(userSvc *corev1.Service) error
}

// Controller hold controller configuration.
type Controller struct {
	clients              k8s.Client
	kubernetesFactory    informers.SharedInformerFactory
	accessFactory        accessInformer.SharedInformerFactory
	specsFactory         specsInformer.SharedInformerFactory
	splitFactory         splitInformer.SharedInformerFactory
	handler              *Handler
	serviceManager       ServiceManager
	configRefreshChan    chan string
	provider             base.Provider
	ignored              k8s.IgnoreWrapper
	smiEnabled           bool
	defaultMode          string
	meshNamespace        string
	tcpStateTable        TCPPortMapper
	lastConfiguration    safe.Safe
	api                  api.Interface
	apiPort              int32
	apiHost              string
	deployLog            deploylog.Interface
	PodLister            listers.PodLister
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	TrafficTargetLister  accessLister.TrafficTargetLister
	HTTPRouteGroupLister specsLister.HTTPRouteGroupLister
	TCPRouteLister       specsLister.TCPRouteLister
	TrafficSplitLister   splitLister.TrafficSplitLister
	minHTTPPort          int32
	maxHTTPPort          int32
	log                  logrus.FieldLogger
}

// MeshControllerConfig holds the configuration of the mesh controller.
type MeshControllerConfig struct {
	SMIEnabled       bool
	DefaultMode      string
	Namespace        string
	IgnoreNamespaces []string
	APIPort          int32
	APIHost          string
	MinTCPPort       int32
	MaxTCPPort       int32
	MinHTTPPort      int32
	MaxHTTPPort      int32
	Log              logrus.FieldLogger
}

// NewMeshController is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients k8s.Client, cfg MeshControllerConfig) (*Controller, error) {
	ignored := k8s.NewIgnored()

	for _, ns := range cfg.IgnoreNamespaces {
		ignored.AddIgnoredNamespace(ns)
	}

	ignored.AddIgnoredService("kubernetes", metav1.NamespaceDefault)
	ignored.AddIgnoredNamespace(metav1.NamespaceSystem)
	ignored.AddIgnoredApps("maesh", "jaeger")

	tcpStateTable, err := k8s.NewTCPPortMapping(clients.GetKubernetesClient(), cfg.Namespace, k8s.TCPStateConfigMapName, cfg.MinTCPPort, cfg.MaxTCPPort)
	if err != nil {
		return nil, err
	}

	c := &Controller{
		clients:       clients,
		ignored:       ignored,
		smiEnabled:    cfg.SMIEnabled,
		defaultMode:   cfg.DefaultMode,
		meshNamespace: cfg.Namespace,
		apiPort:       cfg.APIPort,
		apiHost:       cfg.APIHost,
		tcpStateTable: tcpStateTable,
		minHTTPPort:   cfg.MinHTTPPort,
		maxHTTPPort:   cfg.MaxHTTPPort,
		log:           cfg.Log,
	}

	c.init()

	return c, nil
}

func (c *Controller) init() {
	// Create a new SharedInformerFactory, and register the event handler to informers.
	c.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.GetKubernetesClient(), k8s.ResyncPeriod)
	c.ServiceLister = c.kubernetesFactory.Core().V1().Services().Lister()

	c.serviceManager = NewShadowServiceManager(c.ServiceLister, c.meshNamespace, c.tcpStateTable, c.defaultMode, c.minHTTPPort, c.maxHTTPPort, c.clients.GetKubernetesClient())

	// configRefreshChan is used to trigger configuration refreshes and deploys.
	c.configRefreshChan = make(chan string)
	c.handler = NewHandler(c.ignored, c.serviceManager, c.configRefreshChan)

	c.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Pods().Informer().AddEventHandler(c.handler)

	// Create the base listers
	c.PodLister = c.kubernetesFactory.Core().V1().Pods().Lister()
	c.EndpointsLister = c.kubernetesFactory.Core().V1().Endpoints().Lister()

	c.deployLog = deploylog.NewDeployLog(1000)
	c.api = api.NewAPI(c.apiPort, c.apiHost, &c.lastConfiguration, c.deployLog, c.PodLister, c.meshNamespace)

	if c.smiEnabled {
		// Create new SharedInformerFactories, and register the event handler to informers.
		c.accessFactory = accessInformer.NewSharedInformerFactoryWithOptions(c.clients.GetAccessClient(), k8s.ResyncPeriod)
		c.accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(c.handler)

		c.specsFactory = specsInformer.NewSharedInformerFactoryWithOptions(c.clients.GetSpecsClient(), k8s.ResyncPeriod)
		c.specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(c.handler)
		c.specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(c.handler)

		c.splitFactory = splitInformer.NewSharedInformerFactoryWithOptions(c.clients.GetSplitClient(), k8s.ResyncPeriod)
		c.splitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(c.handler)

		// Create the SMI listers
		c.TrafficTargetLister = c.accessFactory.Access().V1alpha1().TrafficTargets().Lister()
		c.HTTPRouteGroupLister = c.specsFactory.Specs().V1alpha1().HTTPRouteGroups().Lister()
		c.TCPRouteLister = c.specsFactory.Specs().V1alpha1().TCPRoutes().Lister()
		c.TrafficSplitLister = c.splitFactory.Split().V1alpha2().TrafficSplits().Lister()

		c.provider = smi.New(c.defaultMode, c.tcpStateTable, c.ignored, c.ServiceLister, c.EndpointsLister, c.PodLister, c.TrafficTargetLister, c.HTTPRouteGroupLister, c.TCPRouteLister, c.TrafficSplitLister, c.minHTTPPort, c.maxHTTPPort)

		return
	}

	// If SMI is not configured, use the kubernetes provider.
	c.provider = kubernetes.New(c.defaultMode, c.tcpStateTable, c.ignored, c.ServiceLister, c.EndpointsLister, c.minHTTPPort, c.maxHTTPPort)
}

// Run is the main entrypoint for the controller.
func (c *Controller) Run(stopCh <-chan struct{}) error {
	var err error
	// Handle a panic with logging and exiting.
	defer utilruntime.HandleCrash()

	c.log.Debug("Initializing Mesh controller")

	// Start the informers.
	c.startInformers(stopCh, 10*time.Second)

	// Create the mesh services here to ensure that they exist
	c.log.Info("Creating initial mesh services")

	if err = c.createMeshServices(); err != nil {
		c.log.Errorf("could not create mesh services: %v", err)
	}
	// Start the api, and enable the readiness endpoint
	c.api.Start()

	for {
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-stopCh:
			c.log.Info("Shutting down workers")
			return nil
		case message := <-c.configRefreshChan:
			// Reload the configuration
			conf, confErr := c.provider.BuildConfig()
			if confErr != nil {
				return confErr
			}

			if message == k8s.ConfigMessageChanForce || !reflect.DeepEqual(c.lastConfiguration.Get(), conf) {
				c.lastConfiguration.Set(conf)

				if deployErr := c.deployConfiguration(conf); deployErr != nil {
					break
				}

				// Configuration successfully deployed, enable readiness in the api.
				c.api.EnableReadiness()
			}
		case <-timer.C:
			rawCfg := c.lastConfiguration.Get()
			if rawCfg == nil {
				break
			}

			dynCfg, ok := rawCfg.(*dynamic.Configuration)
			if !ok {
				c.log.Error("Received unexpected dynamic configuration, skipping")
				break
			}

			c.log.Debug("Deploying configuration to unready nodes")

			if deployErr := c.deployConfigurationToUnreadyNodes(dynCfg); deployErr != nil {
				break
			}

			// Configuration successfully deployed, enable readiness in the api.
			c.api.EnableReadiness()
		}
	}
}

// startInformers starts the controller informers.
func (c *Controller) startInformers(stopCh <-chan struct{}, syncTimeout time.Duration) {
	// Start the informers with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	c.log.Debug("Starting Informers")
	c.kubernetesFactory.Start(stopCh)

	for t, ok := range c.kubernetesFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			c.log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if c.smiEnabled {
		c.accessFactory.Start(stopCh)

		for t, ok := range c.accessFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				c.log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.specsFactory.Start(stopCh)

		for t, ok := range c.specsFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				c.log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.splitFactory.Start(stopCh)

		for t, ok := range c.splitFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				c.log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}
}

func (c *Controller) createMeshServices() error {
	sel, err := c.ignored.LabelSelector()
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
		if c.ignored.IsIgnored(service.ObjectMeta) {
			continue
		}

		c.log.Debugf("Creating mesh for service: %v", service.Name)

		if err := c.serviceManager.Create(service); err != nil {
			return fmt.Errorf("unable to create mesh service: %w", err)
		}
	}

	return nil
}

// deployConfiguration deploys the configuration to the mesh pods.
func (c *Controller) deployConfiguration(config *dynamic.Configuration) error {
	sel := labels.Everything()

	r, err := labels.NewRequirement("component", selection.Equals, []string{"maesh-mesh"})
	if err != nil {
		return err
	}

	sel = sel.Add(*r)

	podList, err := c.PodLister.Pods(c.meshNamespace).List(sel)
	if err != nil {
		return fmt.Errorf("unable to get pods: %w", err)
	}

	if len(podList) == 0 {
		return fmt.Errorf("unable to find any active mesh pods to deploy config : %+v", config)
	}

	if err := c.deployToPods(podList, config); err != nil {
		return fmt.Errorf("error deploying configuration: %v", err)
	}

	return nil
}

// deployConfigurationToUnreadyNodes deploys the configuration to the mesh pods.
func (c *Controller) deployConfigurationToUnreadyNodes(config *dynamic.Configuration) error {
	sel := labels.Everything()

	r, err := labels.NewRequirement("component", selection.Equals, []string{"maesh-mesh"})
	if err != nil {
		return err
	}

	sel = sel.Add(*r)

	podList, err := c.PodLister.Pods(c.meshNamespace).List(sel)
	if err != nil {
		return fmt.Errorf("unable to get pods: %w", err)
	}

	if len(podList) == 0 {
		return fmt.Errorf("unable to find any active mesh pods to deploy config : %+v", config)
	}

	var unreadyPods []*corev1.Pod

	for _, pod := range podList {
		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				unreadyPods = append(unreadyPods, pod)
				break
			}
		}
	}

	if err := c.deployToPods(unreadyPods, config); err != nil {
		return fmt.Errorf("error deploying configuration: %v", err)
	}

	return nil
}

func (c *Controller) deployToPods(pods []*corev1.Pod, config *dynamic.Configuration) error {
	var errg errgroup.Group

	for _, p := range pods {
		pod := p

		c.log.Debugf("Deploying to pod %s with IP %s", pod.Name, pod.Status.PodIP)

		errg.Go(func() error {
			b := backoff.NewExponentialBackOff()
			b.MaxElapsedTime = 15 * time.Second

			op := func() error {
				return c.deployToPod(pod.Name, pod.Status.PodIP, config)
			}

			return backoff.Retry(safe.OperationWithRecover(op), b)
		})
	}

	return errg.Wait()
}

func (c *Controller) deployToPod(name, ip string, config *dynamic.Configuration) error {
	if name == "" || ip == "" {
		// If there is no name or ip, then just return.
		return fmt.Errorf("pod has no name or IP")
	}

	b, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("unable to marshal configuration: %v", err)
	}

	url := fmt.Sprintf("http://%s:8080/api/providers/rest", ip)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("unable to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	if resp != nil {
		defer resp.Body.Close()

		if _, bodyErr := ioutil.ReadAll(resp.Body); bodyErr != nil {
			c.deployLog.LogDeploy(time.Now(), name, ip, false, fmt.Sprintf("unable to read response body: %v", bodyErr))
			return fmt.Errorf("unable to read response body: %v", bodyErr)
		}

		if resp.StatusCode != http.StatusOK {
			c.deployLog.LogDeploy(time.Now(), name, ip, false, fmt.Sprintf("received non-ok response code: %d", resp.StatusCode))
			return fmt.Errorf("received non-ok response code: %d", resp.StatusCode)
		}
	}

	if err != nil {
		c.deployLog.LogDeploy(time.Now(), name, ip, false, fmt.Sprintf("unable to deploy configuration: %v", err))
		return fmt.Errorf("unable to deploy configuration: %v", err)
	}

	c.deployLog.LogDeploy(time.Now(), name, ip, true, "")
	c.log.Debugf("Successfully deployed configuration to pod (%s:%s)", name, ip)

	return nil
}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "maesh-mesh"
}
