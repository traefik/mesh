package controller

import (
	log "github.com/Sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(obj interface{})
	ObjectDeleted(obj interface{})
	ObjectUpdated(objOld, objNew interface{})
}

// ControllerHandler is an implementation of Handler
type ControllerHandler struct {
	IgnoredNamespaces []string
}

// Init handles any handler initialization
func (c *ControllerHandler) Init() error {
	log.Debugln("TestHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (c *ControllerHandler) ObjectCreated(obj interface{}) {
	// assert the type to a Pod object to pull out relevant data
	switch obj.(type) {
	case *corev1.Service:
		service := obj.(*corev1.Service)
		if Contains(c.IgnoredNamespaces, service.Namespace) {
			return
		}
		log.Debugln("ControllerHandler ObjectCreated with type: *corev1.Service")
		log.Debugf("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
		log.Debugf("    Service Name: %s", service.Name)
		log.Debugf("    Namespace: %s", service.Namespace)

	case *corev1.Endpoints:
		endpoints := obj.(*corev1.Endpoints)
		if Contains(c.IgnoredNamespaces, endpoints.Namespace) {
			return
		}
		log.Debugln("ControllerHandler ObjectCreated with type: *corev1.Endpoints")
		log.Debugf("    ResourceVersion: %s", endpoints.ObjectMeta.ResourceVersion)
		log.Debugf("    Endpoints Name: %s", endpoints.Name)
		log.Debugf("    Namespace: %s", endpoints.Namespace)

	case *corev1.Namespace:
		namespace := obj.(*corev1.Namespace)
		if Contains(c.IgnoredNamespaces, namespace.Name) {
			return
		}
		log.Debugln("ControllerHandler ObjectCreated with type: *corev1.Namespace")
		log.Debugf("    ResourceVersion: %s", namespace.ObjectMeta.ResourceVersion)
		log.Debugf("    Namespace Name: %s", namespace.Name)

	}
}

// ObjectDeleted is called when an object is deleted
func (c *ControllerHandler) ObjectDeleted(obj interface{}) {
	log.Debugln("TestHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (c *ControllerHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debugln("TestHandler.ObjectUpdated")
}
