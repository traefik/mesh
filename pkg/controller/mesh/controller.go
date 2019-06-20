package mesh

import (
	"github.com/containous/i3o/pkg/controller/i3o"
	"github.com/containous/i3o/pkg/k8s"
	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiSplitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type Controller struct {
	serviceController   *i3o.Controller
	smiAccessController *i3o.Controller
	smiSpecsController  *i3o.Controller
	smiSplitController  *i3o.Controller
	handler             *Handler
}

// New is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper) *Controller {
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

	handler := NewHandler(clients, ignored)

	return &Controller{
		handler:             handler,
		serviceController:   i3o.NewController(clients, apiv1.Service{}, ignored, handler),
		smiAccessController: i3o.NewController(clients, smiAccessv1alpha1.TrafficTarget{}, ignored, handler),
		smiSpecsController:  i3o.NewController(clients, smiSpecsv1alpha1.HTTPRouteGroup{}, ignored, handler),
		smiSplitController:  i3o.NewController(clients, smiSplitv1alpha1.TrafficSplit{}, ignored, handler),
	}
}

// Run is the main entrypoint for the controller.
func (m *Controller) Run(stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// run the informer to start listing and watching resources
	go m.serviceController.Run(stopCh)

	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
