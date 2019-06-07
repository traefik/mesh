package meshcontroller

import (
	log "github.com/Sirupsen/logrus"
	"github.com/dtomcej/traefik-mesh-controller/controller"
	"github.com/dtomcej/traefik-mesh-controller/utils"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
)

type MeshController struct {
	serviceController   *controller.Controller
	endpointController  *controller.Controller
	namespaceController *controller.Controller
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object
func NewController(client kubernetes.Interface) *MeshController {
	ignoredNamespaces := []string{metav1.NamespaceSystem, utils.MeshNamespace}

	// Create the new subcontrollers
	sc := controller.NewController(client, apiv1.Service{}, ignoredNamespaces)
	ec := controller.NewController(client, apiv1.Endpoints{}, ignoredNamespaces)
	nc := controller.NewController(client, apiv1.Namespace{}, ignoredNamespaces)

	return &MeshController{
		serviceController:   sc,
		endpointController:  ec,
		namespaceController: nc,
	}

}

// Run is the main entrypoint for the controller
func (m *MeshController) Run(stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Infoln("Initializing Mesh controller")

	// run the informer to start listing and watching resources
	go m.serviceController.Run(stopCh)
	go m.endpointController.Run(stopCh)
	go m.namespaceController.Run(stopCh)

}
