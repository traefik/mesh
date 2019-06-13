package meshcontroller

import (
	"github.com/containous/i3o/controller"
	"github.com/containous/i3o/k8s"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type MeshController struct {
	serviceController *controller.Controller
	handler           *Handler
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object
func NewMeshController(clients *k8s.ClientWrapper) *MeshController {
	ignoredNamespaces := k8s.Namespaces{metav1.NamespaceSystem, k8s.MeshNamespace}
	handler := NewHandler(clients, ignoredNamespaces)

	return &MeshController{
		handler:           handler,
		serviceController: controller.NewController(clients, apiv1.Service{}, ignoredNamespaces, handler),
	}
}

// Run is the main entrypoint for the controller
func (m *MeshController) Run(stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// run the informer to start listing and watching resources
	go m.serviceController.Run(stopCh)

	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
