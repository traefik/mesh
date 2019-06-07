package meshcontroller

import (
	log "github.com/Sirupsen/logrus"
	"github.com/containous/i3o/utils"
	corev1 "k8s.io/api/core/v1"
)

// MeshControllerHandler is an implementation of Handler
type Handler struct {
	IgnoredNamespaces []string
}

func NewHandler(namespaces []string) *Handler {
	return &Handler{
		IgnoredNamespaces: namespaces,
	}
}

// Init handles any handler initialization
func (m *Handler) Init() error {
	log.Debugln("MeshControllerHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (m *Handler) ObjectCreated(obj interface{}) {
	// assert the type to a Pod object to pull out relevant data
	switch obj := obj.(type) {
	case *corev1.Service:
		if utils.Contains(m.IgnoredNamespaces, obj.Namespace) {
			return
		}
		log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Service")
		log.Debugf("    ResourceVersion: %s", obj.ObjectMeta.ResourceVersion)
		log.Debugf("    Service Name: %s", obj.Name)
		log.Debugf("    Namespace: %s", obj.Namespace)

	case *corev1.Endpoints:
		if utils.Contains(m.IgnoredNamespaces, obj.Namespace) {
			return
		}
		log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Endpoints")
		log.Debugf("    ResourceVersion: %s", obj.ObjectMeta.ResourceVersion)
		log.Debugf("    Endpoints Name: %s", obj.Name)
		log.Debugf("    Namespace: %s", obj.Namespace)

	case *corev1.Namespace:
		if utils.Contains(m.IgnoredNamespaces, obj.Name) {
			return
		}
		log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Namespace")
		log.Debugf("    ResourceVersion: %s", obj.ObjectMeta.ResourceVersion)
		log.Debugf("    Namespace Name: %s", obj.Name)

	}
}

// ObjectDeleted is called when an object is deleted
func (m *Handler) ObjectDeleted(obj interface{}) {
	log.Debugln("MeshControllerHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (m *Handler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debugln("MeshControllerHandler.ObjectUpdated")
}
