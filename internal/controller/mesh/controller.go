package mesh

import (
	"time"

	"github.com/containous/i3o/internal/controller/i3o"
	"github.com/containous/i3o/internal/deployer"
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/providers/kubernetes"
	"github.com/containous/i3o/internal/providers/smi"
	"github.com/containous/traefik/pkg/config"
	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiSplitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	serviceController   *i3o.Controller
	smiAccessController *i3o.Controller
	smiSpecsController  *i3o.Controller
	smiSplitController  *i3o.Controller
	handler             *Handler
	messageQueue        workqueue.RateLimitingInterface
	configurationQueue  workqueue.RateLimitingInterface
	kubernetesProvider  *kubernetes.Provider
	smiProvider         *smi.Provider
	deployer            *deployer.Deployer
	ignored             k8s.IgnoreWrapper
	smiEnabled          bool
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper, smiEnabled bool) *Controller {
	ignoredNamespaces := k8s.Namespaces{metav1.NamespaceSystem, k8s.MeshNamespace}
	ignoredServices := k8s.Services{
		{
			Name:      "kubernetes",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ignored := k8s.IgnoreWrapper{
		Namespaces: ignoredNamespaces,
		Services:   ignoredServices,
	}

	// messageQueue is used to process messages from the sub-controllers
	// if cross-controller logic is required
	messageQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	handler := NewHandler(clients, ignored, messageQueue)

	nameService, lwService, objService := newServiceListWatch(clients)

	m := &Controller{
		handler:           handler,
		messageQueue:      messageQueue,
		ignored:           ignored,
		smiEnabled:        smiEnabled,
		serviceController: i3o.NewController(nameService, lwService, objService, corev1.Service{}, ignored, handler),
	}

	if err := m.Init(clients); err != nil {
		log.Errorln("Could not initialize MeshController")
	}

	return m
}

// Init the Controller.
func (m *Controller) Init(clients *k8s.ClientWrapper) error {

	m.kubernetesProvider = kubernetes.New(clients)

	// configurationQueue is used to process configurations from the providers
	// and deal with pushing them to mesh nodes
	m.configurationQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	//Initialize the deployer
	m.deployer = deployer.New(clients, m.configurationQueue)

	if m.smiEnabled {
		m.smiProvider = smi.New(clients)

		nameTrafficTarget, lwTrafficTarget, objTrafficTarget := newTrafficTargetListWatch(clients)
		nameHTTPGroup, lwHTTPGroup, objHTTPGroup := newHTTPRouteGroupListWatch(clients)
		nameTrafficSplit, lwTrafficSplit, objTrafficSplit := newTrafficSplitListWatch(clients)

		m.smiAccessController = i3o.NewController(nameTrafficTarget, lwTrafficTarget, objTrafficTarget, smiAccessv1alpha1.TrafficTarget{}, m.ignored, m.handler)
		m.smiSpecsController = i3o.NewController(nameHTTPGroup, lwHTTPGroup, objHTTPGroup, smiSpecsv1alpha1.HTTPRouteGroup{}, m.ignored, m.handler)
		m.smiSplitController = i3o.NewController(nameTrafficSplit, lwTrafficSplit, objTrafficSplit, smiSplitv1alpha1.TrafficSplit{}, m.ignored, m.handler)

	}

	return nil
}

// Run is the main entrypoint for the controller.
func (m *Controller) Run(stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// run the service controller  to start listing and watching resources
	go m.serviceController.Run(stopCh)

	//run the deployer to deploy configurations
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

	event := item.(i3o.Message)

	switch event.Action {
	case i3o.MessageTypeCreated:
		log.Infof("MeshController.processNextItem: created: %s", event.Key)
		config := m.buildConfigurationFromProviders()
		m.configurationQueue.Add(config)

	case i3o.MessageTypeUpdated:
		log.Infof("MeshController.processNextItem: updated: %s", event.Key)

	case i3o.MessageTypeDeleted:
		log.Infof("MeshController.processNextItem: deleted: %s", event.Key)
	}

	m.messageQueue.Forget(item)

	// keep the worker loop running by returning true if there are queue objects remaining
	return m.messageQueue.Len() > 0
}

func (m *Controller) buildConfigurationFromProviders() *config.Configuration {
	result := m.kubernetesProvider.BuildConfiguration()
	if m.smiEnabled {
		result = mergeConfigurations(result, m.smiProvider.BuildConfiguration())
	}

	return result
}
