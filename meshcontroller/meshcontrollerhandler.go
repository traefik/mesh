package meshcontroller

import (
	log "github.com/Sirupsen/logrus"
	"github.com/dtomcej/traefik-mesh-controller/utils"
	corev1 "k8s.io/api/core/v1"
)

// MeshControllerHandler is an implementation of Handler
type MeshControllerHandler struct {
	IgnoredNamespaces []string
}

func NewMeshControllerHandler(namespaces []string) *MeshControllerHandler {
	return &MeshControllerHandler{
		IgnoredNamespaces: namespaces,
	}
}

// Init handles any handler initialization
func (m *MeshControllerHandler) Init() error {
	log.Debugln("MeshControllerHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (m *MeshControllerHandler) ObjectCreated(obj interface{}) {
	// assert the type to a Pod object to pull out relevant data
	switch obj.(type) {
	case *corev1.Service:
		service := obj.(*corev1.Service)
		if utils.Contains(m.IgnoredNamespaces, service.Namespace) {
			return
		}
		log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Service")
		log.Debugf("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
		log.Debugf("    Service Name: %s", service.Name)
		log.Debugf("    Namespace: %s", service.Namespace)

	case *corev1.Endpoints:
		endpoints := obj.(*corev1.Endpoints)
		if utils.Contains(m.IgnoredNamespaces, endpoints.Namespace) {
			return
		}
		log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Endpoints")
		log.Debugf("    ResourceVersion: %s", endpoints.ObjectMeta.ResourceVersion)
		log.Debugf("    Endpoints Name: %s", endpoints.Name)
		log.Debugf("    Namespace: %s", endpoints.Namespace)

	case *corev1.Namespace:
		namespace := obj.(*corev1.Namespace)
		if utils.Contains(m.IgnoredNamespaces, namespace.Name) {
			return
		}
		log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Namespace")
		log.Debugf("    ResourceVersion: %s", namespace.ObjectMeta.ResourceVersion)
		log.Debugf("    Namespace Name: %s", namespace.Name)

	}
}

// ObjectDeleted is called when an object is deleted
func (m *MeshControllerHandler) ObjectDeleted(obj interface{}) {
	log.Debugln("MeshControllerHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (m *MeshControllerHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debugln("MeshControllerHandler.ObjectUpdated")
}
