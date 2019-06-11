package meshcontroller

import (
	"github.com/containous/i3o/controller"
	"github.com/containous/i3o/utils"
	traefik_v1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type MeshController struct {
	serviceController   *controller.Controller
	endpointController  *controller.Controller
	namespaceController *controller.Controller
	crdController       *controller.Controller
	handler             *Handler
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object
func NewMeshController() *MeshController {
	return &MeshController{}
}

// Init prepares the controller by creating the required subcontrollers.
func (m *MeshController) Init(clients *utils.ClientWrapper) {
	ignoredNamespaces := []string{metav1.NamespaceSystem, utils.MeshNamespace}

	m.handler = NewHandler(ignoredNamespaces)
	// Create the new subcontrollers
	m.serviceController = controller.NewController(clients, apiv1.Service{}, ignoredNamespaces, m.handler)
	m.endpointController = controller.NewController(clients, apiv1.Endpoints{}, ignoredNamespaces, m.handler)
	m.namespaceController = controller.NewController(clients, apiv1.Namespace{}, ignoredNamespaces, m.handler)
	m.crdController = controller.NewController(clients, traefik_v1alpha1.IngressRoute{}, ignoredNamespaces, m.handler)
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
	go m.crdController.Run(stopCh)

	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
