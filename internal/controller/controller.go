package controller

import (
	"fmt"
	"time"

	"github.com/containous/i3o/internal/deployer"
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/message"
	"github.com/containous/i3o/internal/providers/kubernetes"
	"github.com/containous/i3o/internal/providers/smi"
	"github.com/containous/traefik/pkg/config"
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
	messageQueue       workqueue.RateLimitingInterface
	configurationQueue workqueue.RateLimitingInterface
	kubernetesProvider *kubernetes.Provider
	smiProvider        *smi.Provider
	deployer           *deployer.Deployer
	ignored            k8s.IgnoreWrapper
	smiEnabled         bool
	traefikConfig      *config.Configuration
	defaultMode        string
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper, smiEnabled bool, defaultMode string) *Controller {
	ignored := k8s.NewIgnored()

	// messageQueue is used to process messages from the sub-controllers
	// if cross-controller logic is required
	messageQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	handler := NewHandler(ignored, messageQueue)

	m := &Controller{
		clients:      clients,
		handler:      handler,
		messageQueue: messageQueue,
		ignored:      ignored,
		smiEnabled:   smiEnabled,
		defaultMode:  defaultMode,
	}

	if err := m.Init(); err != nil {
		log.Errorln("Could not initialize MeshController")
	}

	return m
}

// Init the Controller.
func (m *Controller) Init() error {
	// Create a new SharedInformerFactory, and register the event handler to informers.
	m.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(m.clients.KubeClient, k8s.ResyncPeriod)
	m.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(m.handler)
	m.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(m.handler)

	// Create a new SharedInformerFactory, and register the event handler to informers.
	m.meshFactory = informers.NewSharedInformerFactoryWithOptions(m.clients.KubeClient, k8s.ResyncPeriod, informers.WithNamespace(k8s.MeshNamespace))
	m.meshFactory.Core().V1().Pods().Informer().AddEventHandler(m.handler)

	m.kubernetesProvider = kubernetes.New(m.clients, m.defaultMode)

	// configurationQueue is used to process configurations from the providers
	// and deal with pushing them to mesh nodes
	m.configurationQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Initialize the deployer.
	m.deployer = deployer.New(m.clients, m.configurationQueue)

	if m.smiEnabled {
		m.smiProvider = smi.New(m.clients)

		// Create new SharedInformerFactories, and register the event handler to informers.
		m.smiAccessFactory = smiAccessExternalversions.NewSharedInformerFactoryWithOptions(m.clients.SmiAccessClient, k8s.ResyncPeriod)
		m.smiAccessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(m.handler)

		m.smiSpecsFactory = smiSpecsExternalversions.NewSharedInformerFactoryWithOptions(m.clients.SmiSpecsClient, k8s.ResyncPeriod)
		m.smiSpecsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(m.handler)

		m.smiSplitFactory = smiSplitExternalversions.NewSharedInformerFactoryWithOptions(m.clients.SmiSplitClient, k8s.ResyncPeriod)
		m.smiSplitFactory.Split().V1alpha1().TrafficSplits().Informer().AddEventHandler(m.handler)
	}

	// Initialize an empty configuration
	m.traefikConfig = &config.Configuration{
		HTTP: &config.HTTPConfiguration{
			Routers:  map[string]*config.Router{},
			Services: map[string]*config.Service{},
		},
		TCP: &config.TCPConfiguration{
			Routers:  map[string]*config.TCPRouter{},
			Services: map[string]*config.TCPService{},
		},
	}

	return nil
}

// Run is the main entrypoint for the controller.
func (m *Controller) Run(stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// Start the informers
	m.kubernetesFactory.Start(stopCh)
	for t, ok := range m.kubernetesFactory.WaitForCacheSync(stopCh) {
		if !ok {
			log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	m.meshFactory.Start(stopCh)
	for t, ok := range m.meshFactory.WaitForCacheSync(stopCh) {
		if !ok {
			log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if m.smiEnabled {
		m.smiAccessFactory.Start(stopCh)
		for t, ok := range m.smiAccessFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		m.smiSpecsFactory.Start(stopCh)
		for t, ok := range m.smiSpecsFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		m.smiSplitFactory.Start(stopCh)
		for t, ok := range m.smiSplitFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}

	// run the deployer to deploy configurations
	go m.deployer.Run(stopCh)

	// run the runWorker method every second with a stop channel
	wait.Until(m.runWorker, time.Second, stopCh)

	<-stopCh
	log.Info("Shutting down workers")

	return nil
}

// runWorker executes the loop to process new items added to the queue
func (m *Controller) runWorker() {
	log.Debug("MeshController.runWorker: starting")

	// invoke processNextMessage to fetch and consume the next
	// message put in the queue
	for m.processNextMessage() {
		log.Debugf("MeshController.runWorker: processing next item")
	}

	log.Debugf("MeshController.runWorker: completed")
}

// processNextConfiguration retrieves each queued item and takes the
// necessary handler action.
func (m *Controller) processNextMessage() bool {
	log.Debugf("MeshController Waiting for next item to process...")

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	item, quit := m.messageQueue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer m.messageQueue.Done(item)

	event := item.(message.Message)

	switch event.Action {
	case message.TypeCreated:
		m.processCreatedMessage(event)
	case message.TypeUpdated:
		m.processUpdatedMessage(event)
	case message.TypeDeleted:
		m.processDeletedMessage(event)
	}

	m.messageQueue.Forget(item)

	// keep the worker loop running by returning true if there are queue objects remaining
	return m.messageQueue.Len() > 0
}

func (m *Controller) buildConfigurationFromProviders(event message.Message) {
	m.kubernetesProvider.BuildConfiguration(event, m.traefikConfig.DeepCopy())
	if m.smiEnabled {
		m.smiProvider.BuildConfiguration()
	}
}

func (m *Controller) processCreatedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		if m.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectCreated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		log.Debugf("Creating associated mesh service for service: %s/%s", obj.Namespace, obj.Name)
		if _, err := m.createMeshService(obj); err != nil {
			log.Errorf("Could not create mesh service: %v", err)
			return
		}

	case *corev1.Endpoints:
		log.Debugf("MeshController ObjectCreated with type: *corev1.Endpoints: %s/%s, skipping...", obj.Namespace, obj.Name)
		return

	case *corev1.Pod:
		log.Debugf("MeshController ObjectCreated with type: *corev1.Pod: %s/%s", obj.Namespace, obj.Name)
		if isMeshPod(obj) {
			// Re-Deploy configuration to the created mesh pod.
			m.deployer.DeployToPod(obj.Name, obj.Status.PodIP, m.traefikConfig.DeepCopy())
		}
		return
	}

	m.buildConfigurationFromProviders(event)
	m.configurationQueue.Add(message.Config{
		Config: m.traefikConfig.DeepCopy(),
	})
}

func (m *Controller) processUpdatedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		if m.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectUpdated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)
		oldService := event.OldObject.(*corev1.Service)
		if _, err := m.updateMeshService(oldService, obj); err != nil {
			log.Errorf("Could not update mesh service: %v", err)
			return
		}

	case *corev1.Endpoints:
		if m.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectUpdated with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)

	case *corev1.Pod:
		log.Debugf("MeshController ObjectUpdated with type: *corev1.Pod: %s/%s, skipping...", obj.Namespace, obj.Name)
		return

	}

	m.buildConfigurationFromProviders(event)
	m.configurationQueue.Add(message.Config{
		Config: m.traefikConfig.DeepCopy(),
	})

}

func (m *Controller) processDeletedMessage(event message.Message) {
	// assert the type to an object to pull out relevant data
	switch obj := event.Object.(type) {
	case *corev1.Service:
		// assert the type to an object to pull out relevant data
		if m.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectDeleted with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		if err := m.deleteMeshService(obj.Name, obj.Namespace); err != nil {
			log.Errorf("Could not delete mesh service: %v", err)
			return
		}

	case *corev1.Endpoints:
		if m.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectDeleted with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)

	case *corev1.Pod:
		log.Debugf("MeshController ObjectDeleted with type: *corev1.Pod: %s/%s, skipping...", obj.Namespace, obj.Name)
		return
	}

	m.buildConfigurationFromProviders(event)
	m.configurationQueue.Add(message.Config{
		Config: m.traefikConfig.DeepCopy(),
	})

}

func (m *Controller) createMeshService(service *corev1.Service) (*corev1.Service, error) {
	meshServiceName := userServiceToMeshServiceName(service.Name, service.Namespace)
	meshServiceInstance, exists, err := m.clients.GetService(k8s.MeshNamespace, meshServiceName)
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
				Namespace: k8s.MeshNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: ports,
				Selector: map[string]string{
					"component": "i3o-mesh",
				},
			},
		}

		return m.clients.CreateService(svc)
	}

	return meshServiceInstance, nil
}

func (m *Controller) deleteMeshService(serviceName, serviceNamespace string) error {
	meshServiceName := userServiceToMeshServiceName(serviceName, serviceNamespace)
	_, exists, err := m.clients.GetService(k8s.MeshNamespace, meshServiceName)
	if err != nil {
		return err
	}

	if exists {
		// Service exists, delete
		if err := m.clients.DeleteService(k8s.MeshNamespace, meshServiceName); err != nil {
			return err
		}
		log.Debugf("Deleted service: %s/%s", k8s.MeshNamespace, meshServiceName)
	}

	return nil
}

// updateMeshService updates the mesh service based on an old/new user service, and returns the updated mesh service
func (m *Controller) updateMeshService(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error) {
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	meshServiceName := userServiceToMeshServiceName(oldUserService.Name, oldUserService.Namespace)

	var updatedSvc *corev1.Service
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, exists, err := m.clients.GetService(k8s.MeshNamespace, meshServiceName)
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

			updatedSvc, err = m.clients.UpdateService(newService)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if retryErr != nil {
		return nil, fmt.Errorf("unable to update service %q: %v", meshServiceName, retryErr)
	}

	log.Debugf("Updated service: %s/%s", k8s.MeshNamespace, meshServiceName)
	return updatedSvc, nil

}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "i3o-mesh"
}

// userServiceToMeshServiceName converts a User service with a namespace to a traefik-mesh service name.
func userServiceToMeshServiceName(serviceName string, namespace string) string {
	return fmt.Sprintf("traefik-%s-%s", serviceName, namespace)
}
