package controller

import (
	"fmt"
	"time"

	"github.com/containous/maesh/internal/deployer"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/message"
	"github.com/containous/maesh/internal/providers/kubernetes"
	"github.com/containous/maesh/internal/providers/smi"
	"github.com/containous/traefik/pkg/config/dynamic"
	smiAccessExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	smiSpecsExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	smiSplitExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	clients            *k8s.ClientWrapper
	kubernetesFactory  informers.SharedInformerFactory
	meshFactory        informers.SharedInformerFactory
	smiAccessFactory   smiAccessExternalversions.SharedInformerFactory
	smiSpecsFactory    smiSpecsExternalversions.SharedInformerFactory
	smiSplitFactory    smiSplitExternalversions.SharedInformerFactory
	handler            *Handler
	meshHandler        *Handler
	messageQueue       workqueue.RateLimitingInterface
	configurationQueue workqueue.RateLimitingInterface
	kubernetesProvider *kubernetes.Provider
	smiProvider        *smi.Provider
	deployer           *deployer.Deployer
	ignored            k8s.IgnoreWrapper
	smiEnabled         bool
	traefikConfig      *dynamic.Configuration
	defaultMode        string
	meshNamespace      string
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper, smiEnabled bool, defaultMode string, meshNamespace string) *Controller {
	ignored := k8s.NewIgnored(meshNamespace)

	// messageQueue is used to process messages from the sub-controllers
	// if cross-controller logic is required
	messageQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	handler := NewHandler(ignored, messageQueue)
	// Create a new mesh handler to handle mesh events (pods)
	meshHandler := NewHandler(ignored.WithoutMesh(), messageQueue)

	c := &Controller{
		clients:       clients,
		handler:       handler,
		meshHandler:   meshHandler,
		messageQueue:  messageQueue,
		ignored:       ignored,
		smiEnabled:    smiEnabled,
		defaultMode:   defaultMode,
		meshNamespace: meshNamespace,
	}

	if err := c.Init(); err != nil {
		log.Errorln("Could not initialize MeshController")
	}

	return c
}

// Init the Controller.
func (c *Controller) Init() error {
	// Create a new SharedInformerFactory, and register the event handler to informers.
	c.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.KubeClient, k8s.ResyncPeriod)
	c.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(c.handler)

	// Create a new SharedInformerFactory, and register the event handler to informers.
	c.meshFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.KubeClient,
		k8s.ResyncPeriod,
		informers.WithNamespace(c.meshNamespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = "component==maesh-mesh"
		}),
	)
	c.meshFactory.Core().V1().Pods().Informer().AddEventHandler(c.meshHandler)

	c.kubernetesProvider = kubernetes.New(c.clients, c.defaultMode, c.meshNamespace)

	// configurationQueue is used to process configurations from the providers
	// and deal with pushing them to mesh nodes
	c.configurationQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Initialize the deployer.
	c.deployer = deployer.New(c.clients, c.configurationQueue, c.meshNamespace)

	// Initialize an empty configuration with a readinesscheck so that configs deployed to nodes mark them as ready.
	c.traefikConfig = createBaseConfigWithReadiness()

	if c.smiEnabled {
		c.smiProvider = smi.New(c.clients, c.defaultMode, c.meshNamespace, c.ignored)

		// Create new SharedInformerFactories, and register the event handler to informers.
		c.smiAccessFactory = smiAccessExternalversions.NewSharedInformerFactoryWithOptions(c.clients.SmiAccessClient, k8s.ResyncPeriod)
		c.smiAccessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(c.handler)

		c.smiSpecsFactory = smiSpecsExternalversions.NewSharedInformerFactoryWithOptions(c.clients.SmiSpecsClient, k8s.ResyncPeriod)
		c.smiSpecsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(c.handler)

		c.smiSplitFactory = smiSplitExternalversions.NewSharedInformerFactoryWithOptions(c.clients.SmiSplitClient, k8s.ResyncPeriod)
		c.smiSplitFactory.Split().V1alpha1().TrafficSplits().Informer().AddEventHandler(c.handler)

		// Initialize the base configuration with the base SMI middleware
		addBaseSMIMiddlewares(c.traefikConfig)
	}

	return nil
}

// Run is the main entrypoint for the controller.
func (c *Controller) Run(stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// Start the informers
	c.kubernetesFactory.Start(stopCh)
	for t, ok := range c.kubernetesFactory.WaitForCacheSync(stopCh) {
		if !ok {
			log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	c.meshFactory.Start(stopCh)
	for t, ok := range c.meshFactory.WaitForCacheSync(stopCh) {
		if !ok {
			log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if c.smiEnabled {
		c.smiAccessFactory.Start(stopCh)
		for t, ok := range c.smiAccessFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.smiSpecsFactory.Start(stopCh)
		for t, ok := range c.smiSpecsFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.smiSplitFactory.Start(stopCh)
		for t, ok := range c.smiSplitFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}

	// run the deployer to deploy configurations
	go c.deployer.Run(stopCh)

	// run the runWorker method every second with a stop channel
	wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
	log.Info("Shutting down workers")

	return nil
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	log.Debug("MeshController.runWorker: starting")

	// invoke processNextMessage to fetch and consume the next
	// message put in the queue
	for c.processNextMessage() {
		log.Debugf("MeshController.runWorker: processing next item")
	}

	log.Debugf("MeshController.runWorker: completed")
}

// processNextConfiguration retrieves each queued item and takes the
// necessary handler action.
func (c *Controller) processNextMessage() bool {
	log.Debugf("MeshController Waiting for next item to process...")

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	item, quit := c.messageQueue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.messageQueue.Done(item)

	event := item.(message.Message)

	switch event.Action {
	case message.TypeCreated:
		c.processCreatedMessage(event)
	case message.TypeUpdated:
		c.processUpdatedMessage(event)
	case message.TypeDeleted:
		c.processDeletedMessage(event)
	}

	c.messageQueue.Forget(item)

	// keep the worker loop running by returning true if there are queue objects remaining
	return c.messageQueue.Len() > 0
}

func (c *Controller) buildConfigurationFromProviders(event message.Message) {
	if c.smiEnabled {
		c.smiProvider.BuildConfiguration(event, c.traefikConfig)
		return
	}
	c.kubernetesProvider.BuildConfiguration(event, c.traefikConfig)
}

func (c *Controller) processCreatedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		if c.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectCreated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		log.Debugf("Creating associated mesh service for service: %s/%s", obj.Namespace, obj.Name)
		service, err := c.createMeshService(obj)
		if err != nil {
			log.Errorf("Could not create mesh service: %v", err)
			return
		}

		if err = c.setUserServiceExternalIP(obj, service.Spec.ClusterIP); err != nil {
			log.Errorf("Could not update user service with externalIP: %v", err)
		}

	case *corev1.Endpoints:
		log.Debugf("MeshController ObjectCreated with type: *corev1.Endpoints: %s/%s, skipping...", obj.Namespace, obj.Name)
		return

	case *corev1.Pod:
		log.Debugf("MeshController ObjectCreated with type: *corev1.Pod: %s/%s", obj.Namespace, obj.Name)
		if isMeshPod(obj) {
			// Re-Deploy configuration to the created mesh pod.
			msg := message.BuildNewConfigWithVersion(c.traefikConfig)
			// Don't deploy if name or IP are unassigned.
			if obj.Name != "" && obj.Status.PodIP != "" {
				c.deployer.DeployToPod(obj.Name, obj.Status.PodIP, msg.Config)
			}
		}
		return
	}

	c.buildConfigurationFromProviders(event)
	c.configurationQueue.Add(message.BuildNewConfigWithVersion(c.traefikConfig))
}

func (c *Controller) processUpdatedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		if c.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectUpdated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)
		oldService := event.OldObject.(*corev1.Service)
		if _, err := c.updateMeshService(oldService, obj); err != nil {
			log.Errorf("Could not update mesh service: %v", err)
			return
		}

	case *corev1.Endpoints:
		if c.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectUpdated with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)

	case *corev1.Pod:
		log.Debugf("MeshController ObjectUpdated with type: *corev1.Pod: %s/%s", obj.Namespace, obj.Name)
		if isMeshPod(obj) {
			// Re-Deploy configuration to the updated mesh pod.
			msg := message.BuildNewConfigWithVersion(c.traefikConfig)
			// Don't deploy if name or IP are unassigned.
			if obj.Name != "" && obj.Status.PodIP != "" {
				c.deployer.DeployToPod(obj.Name, obj.Status.PodIP, msg.Config)
			}
		}
		return

	}

	c.buildConfigurationFromProviders(event)
	c.configurationQueue.Add(message.BuildNewConfigWithVersion(c.traefikConfig))

}

func (c *Controller) processDeletedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		// assert the type to an object to pull out relevant data
		if c.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectDeleted with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		if err := c.deleteMeshService(obj.Name, obj.Namespace); err != nil {
			log.Errorf("Could not delete mesh service: %v", err)
			return
		}

	case *corev1.Endpoints:
		if c.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectDeleted with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)

	case *corev1.Pod:
		log.Debugf("MeshController ObjectDeleted with type: *corev1.Pod: %s/%s, skipping...", obj.Namespace, obj.Name)
		return
	}

	c.buildConfigurationFromProviders(event)
	c.configurationQueue.Add(message.BuildNewConfigWithVersion(c.traefikConfig))

}

func (c *Controller) createMeshService(service *corev1.Service) (*corev1.Service, error) {
	meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)
	meshServiceInstance, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
	if err != nil {
		return nil, err
	}

	if !exists {
		// Mesh service does not exist.
		var ports []corev1.ServicePort

		for id, sp := range service.Spec.Ports {
			if sp.Protocol != corev1.ProtocolTCP {
				log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, service.Namespace, service.Name)
				continue
			}

			meshPort := corev1.ServicePort{
				Name:       sp.Name,
				Port:       sp.Port,
				TargetPort: intstr.FromInt(5000 + id),
			}

			ports = append(ports, meshPort)
		}

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: c.meshNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: ports,
				Selector: map[string]string{
					"component": "maesh-mesh",
				},
			},
		}

		return c.clients.CreateService(svc)
	}

	return meshServiceInstance, nil
}

func (c *Controller) deleteMeshService(serviceName, serviceNamespace string) error {
	meshServiceName := c.userServiceToMeshServiceName(serviceName, serviceNamespace)
	_, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
	if err != nil {
		return err
	}

	if exists {
		// Service exists, delete
		if err := c.clients.DeleteService(c.meshNamespace, meshServiceName); err != nil {
			return err
		}
		log.Debugf("Deleted service: %s/%s", c.meshNamespace, meshServiceName)
	}

	return nil
}

// updateMeshService updates the mesh service based on an old/new user service, and returns the updated mesh service
func (c *Controller) updateMeshService(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error) {
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	meshServiceName := c.userServiceToMeshServiceName(oldUserService.Name, oldUserService.Namespace)

	var updatedSvc *corev1.Service
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
		if err != nil {
			return err
		}

		if exists {
			var ports []corev1.ServicePort

			for id, sp := range newUserService.Spec.Ports {
				if sp.Protocol != corev1.ProtocolTCP {
					log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, newUserService.Namespace, newUserService.Name)
					continue
				}

				meshPort := corev1.ServicePort{
					Name:       sp.Name,
					Port:       sp.Port,
					TargetPort: intstr.FromInt(5000 + id),
				}

				ports = append(ports, meshPort)
			}

			newService := service.DeepCopy()
			newService.Spec.Ports = ports

			updatedSvc, err = c.clients.UpdateService(newService)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if retryErr != nil {
		return nil, fmt.Errorf("unable to update service %q: %v", meshServiceName, retryErr)
	}

	log.Debugf("Updated service: %s/%s", c.meshNamespace, meshServiceName)
	return updatedSvc, nil

}

// setUserServiceExternalIP sets the externalIP of the user's service to provide a DNS record.
func (c *Controller) setUserServiceExternalIP(userService *corev1.Service, ip string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newService := userService.DeepCopy()
		newService.Spec.ExternalIPs = []string{ip}

		_, err := c.clients.UpdateService(newService)
		return err
	})
}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "maesh-mesh"
}

// userServiceToMeshServiceName converts a User service with a namespace to a mesh service name.
func (c *Controller) userServiceToMeshServiceName(serviceName string, namespace string) string {
	return fmt.Sprintf("%s-%s-%s", c.meshNamespace, serviceName, namespace)
}

func createBaseConfigWithReadiness() *dynamic.Configuration {
	return &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"readiness": {
					Rule:        "Path(`/ping`)",
					EntryPoints: []string{"readiness"},
					Service:     "readiness",
				},
			},
			Services: map[string]*dynamic.Service{
				"readiness": {
					LoadBalancer: &dynamic.LoadBalancerService{
						Servers: []dynamic.Server{
							{
								URL: "http://127.0.0.1:8080",
							},
						},
					},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{},
		},
		TCP: &dynamic.TCPConfiguration{
			Routers:  map[string]*dynamic.TCPRouter{},
			Services: map[string]*dynamic.TCPService{},
		},
	}
}

func addBaseSMIMiddlewares(config *dynamic.Configuration) {
	blockAll := &dynamic.Middleware{
		IPWhiteList: &dynamic.IPWhiteList{
			SourceRange: []string{"255.255.255.255"},
		},
	}

	config.HTTP.Middlewares[k8s.BlockAllMiddlewareKey] = blockAll
}
