package meshcontroller

import (
	"github.com/containous/i3o/controller"
	"github.com/containous/i3o/utils"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
)

type MeshController struct {
	serviceController   *controller.Controller
	endpointController  *controller.Controller
	namespaceController *controller.Controller
	handler             *Handler
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object
func NewMeshController() *MeshController {
	return &MeshController{}
}

// Init prepares the controller by creating the required subcontrollers.
func (m *MeshController) Init(client kubernetes.Interface) {
	ignoredNamespaces := []string{metav1.NamespaceSystem, utils.MeshNamespace}

	m.handler = NewHandler(ignoredNamespaces)
	// Create the new subcontrollers
	m.serviceController = controller.NewController(client, apiv1.Service{}, ignoredNamespaces, m.handler)
	m.endpointController = controller.NewController(client, apiv1.Endpoints{}, ignoredNamespaces, m.handler)
	m.namespaceController = controller.NewController(client, apiv1.Namespace{}, ignoredNamespaces, m.handler)
}

// Run is the main entrypoint for the controller
func (m *MeshController) Run(stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// run the informer to start listing and watching resources
	go m.serviceController.Run(stopCh)
	go m.endpointController.Run(stopCh)
	go m.namespaceController.Run(stopCh)

	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
