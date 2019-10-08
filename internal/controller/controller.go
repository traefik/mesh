package controller

import (
	"fmt"
	"strconv"
	"time"

	"github.com/containous/maesh/internal/deployer"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/message"
	"github.com/containous/maesh/internal/providers/base"
	"github.com/containous/maesh/internal/providers/kubernetes"
	"github.com/containous/maesh/internal/providers/smi"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
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

// Controller hold controller configuration.
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
	tcpStateTable      *k8s.State
}

// NewMeshController is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper, smiEnabled bool, defaultMode string, meshNamespace string, ignoreNamespaces []string) *Controller {
	ignored := k8s.NewIgnored(meshNamespace, ignoreNamespaces)

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

	c.tcpStateTable = &k8s.State{Table: make(map[int]*k8s.ServiceWithPort)}
	c.kubernetesProvider = kubernetes.New(c.clients, c.defaultMode, c.meshNamespace, c.tcpStateTable, c.ignored)

	// configurationQueue is used to process configurations from the providers
	// and deal with pushing them to mesh nodes
	c.configurationQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Initialize the deployer.
	c.deployer = deployer.New(c.clients, c.configurationQueue, c.meshNamespace)

	// Initialize an empty configuration with a readinesscheck so that configs deployed to nodes mark them as ready.
	c.traefikConfig = base.CreateBaseConfigWithReadiness()

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
	var err error
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

	// Load the state from the TCP State Configmap before running.
	c.tcpStateTable, err = c.loadTCPStateTable()
	if err != nil {
		log.Errorf("encountered error loading TCP state table: %v", err)
	}

	// Create the mesh services here to ensure that they exist
	log.Info("Creating initial mesh services")
	if err := c.createMeshServices(); err != nil {
		log.Errorf("could not create mesh services: %v", err)
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
	// invoke processNextMessage to fetch and consume the next
	// message put in the queue
	for c.processNextMessage() {
	}
}

// processNextConfiguration retrieves each queued item and takes the
// necessary handler action.
func (c *Controller) processNextMessage() bool {
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

func (c *Controller) buildConfigurationFromProviders() {
	// Create all mesh services
	if err := c.createMeshServices(); err != nil {
		log.Errorf("could not create mesh services: %v", err)
	}

	var config *dynamic.Configuration
	var err error

	if c.smiEnabled {
		config, err = c.smiProvider.BuildConfig()
	} else {
		config, err = c.kubernetesProvider.BuildConfig()
	}
	if err != nil {
		log.Errorf("unable to build configuration: %v", err)
	}
	c.traefikConfig = config
}

func (c *Controller) processCreatedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		if c.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

	case *corev1.Endpoints:
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

	c.buildConfigurationFromProviders()
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

	c.buildConfigurationFromProviders()
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
		return
	}

	c.buildConfigurationFromProviders()
	c.configurationQueue.Add(message.BuildNewConfigWithVersion(c.traefikConfig))
}

func (c *Controller) createMeshServices() error {
	services, err := c.clients.GetServices(metav1.NamespaceAll)
	if err != nil {
		return fmt.Errorf("unable to get services: %v", err)
	}

	for _, service := range services {
		if c.ignored.Ignored(service.Name, service.Namespace) {
			continue
		}
		log.Debugf("Creating mesh for service: %v", service.Name)
		meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)
		for _, subservice := range services {
			// If there is already a mesh service created, don't bother recreating
			if subservice.Name == meshServiceName && subservice.Namespace == c.meshNamespace {
				continue
			}
		}
		log.Infof("Creating associated mesh service: %s", meshServiceName)
		if err := c.createMeshService(service); err != nil {
			return fmt.Errorf("unable to get create mesh service: %v", err)
		}
	}
	return nil
}

func (c *Controller) createMeshService(service *corev1.Service) error {
	meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)
	log.Debugf("Creating mesh service: %s", meshServiceName)
	_, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
	if err != nil {
		return err
	}

	if !exists {
		// Mesh service does not exist.
		var ports []corev1.ServicePort

		serviceMode := service.Annotations[k8s.AnnotationServiceType]
		if serviceMode == "" {
			serviceMode = c.defaultMode
		}

		for id, sp := range service.Spec.Ports {
			if sp.Protocol != corev1.ProtocolTCP {
				log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, service.Namespace, service.Name)
				continue
			}

			targetPort := intstr.FromInt(5000 + id)
			if serviceMode == k8s.ServiceTypeTCP {
				targetPort = intstr.FromInt(c.getTCPPortFromState(service.Name, service.Namespace, sp.Port))
			}

			if targetPort.IntVal == 0 {
				log.Errorf("Could not get TCP Port for service: %s with service port: %v", service.Name, sp)
				continue
			}

			meshPort := corev1.ServicePort{
				Name:       sp.Name,
				Port:       sp.Port,
				TargetPort: targetPort,
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

		_, err = c.clients.CreateService(svc)
	}

	return err
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

			serviceMode := newUserService.Annotations[k8s.AnnotationServiceType]
			if serviceMode == "" {
				serviceMode = c.defaultMode
			}

			for id, sp := range newUserService.Spec.Ports {
				if sp.Protocol != corev1.ProtocolTCP {
					log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, newUserService.Namespace, newUserService.Name)
					continue
				}

				targetPort := intstr.FromInt(5000 + id)
				if serviceMode == k8s.ServiceTypeTCP {
					targetPort = intstr.FromInt(c.getTCPPortFromState(newUserService.Name, newUserService.Namespace, sp.Port))
				}
				meshPort := corev1.ServicePort{
					Name:       sp.Name,
					Port:       sp.Port,
					TargetPort: targetPort,
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

// userServiceToMeshServiceName converts a User service with a namespace to a mesh service name.
func (c *Controller) userServiceToMeshServiceName(serviceName string, namespace string) string {
	return fmt.Sprintf("%s-%s-%s", c.meshNamespace, serviceName, namespace)
}

func (c *Controller) loadTCPStateTable() (*k8s.State, error) {
	result := c.tcpStateTable
	if result == nil {
		result = &k8s.State{Table: make(map[int]*k8s.ServiceWithPort)}
	}

	configMap, exists, err := c.clients.GetConfigMap(c.meshNamespace, k8s.TCPStateConfigMapName)
	if err != nil {
		return result, err
	}

	if !exists {
		return result, fmt.Errorf("TCP State Table configmap does not exist")
	}

	if len(configMap.Data) > 0 {
		for k, v := range configMap.Data {
			port, err := strconv.Atoi(k)
			if err != nil {
				continue
			}
			name, namespace, servicePort, err := k8s.ParseServiceNamePort(v)
			if err != nil {
				continue
			}
			result.Table[port] = &k8s.ServiceWithPort{
				Name:      name,
				Namespace: namespace,
				Port:      servicePort,
			}
		}
	}
	return result, nil
}

func (c *Controller) getTCPPortFromState(serviceName, serviceNamespace string, servicePort int32) int {
	for port, v := range c.tcpStateTable.Table {
		if v.Name == serviceName && v.Namespace == serviceNamespace && v.Port == servicePort {
			return port
		}
	}
	log.Debugf("No match found for %s/%s %d - Add a new port", serviceName, serviceNamespace, servicePort)
	// No Match, add new port
	for i := 10000; true; i++ {
		if _, exists := c.tcpStateTable.Table[i]; exists {
			// Port used
			continue
		}
		c.tcpStateTable.Table[i] = &k8s.ServiceWithPort{
			Name:      serviceName,
			Namespace: serviceNamespace,
			Port:      servicePort,
		}

		if err := c.saveTCPStateTable(); err != nil {
			log.Errorf("unable to save TCP state table config map: %v", err)
			return 0
		}
		return i
	}
	return 0
}

func (c *Controller) saveTCPStateTable() error {
	configMap, exists, err := c.clients.GetConfigMap(c.meshNamespace, k8s.TCPStateConfigMapName)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("TCP State Table configmap does not exist")
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newConfigMap := configMap.DeepCopy()

		if newConfigMap.Data == nil {
			newConfigMap.Data = make(map[string]string)
		}
		for k, v := range c.tcpStateTable.Table {
			key := strconv.Itoa(k)
			value := k8s.ServiceNamePortToString(v.Name, v.Namespace, v.Port)
			newConfigMap.Data[key] = value
		}
		_, err := c.clients.UpdateConfigMap(newConfigMap)
		return err
	})
}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "maesh-mesh"
}

func addBaseSMIMiddlewares(config *dynamic.Configuration) {
	blockAll := &dynamic.Middleware{
		IPWhiteList: &dynamic.IPWhiteList{
			SourceRange: []string{"255.255.255.255"},
		},
	}

	config.HTTP.Middlewares[k8s.BlockAllMiddlewareKey] = blockAll
}
