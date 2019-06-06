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
type ControllerHandler struct{}

// Init handles any handler initialization
func (t *ControllerHandler) Init() error {
	log.Info("TestHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (t *ControllerHandler) ObjectCreated(obj interface{}) {
	// assert the type to a Pod object to pull out relevant data
	switch obj.(type) {
	case *corev1.Service:
		log.Infoln("ControllerHandler ObjectCreated with type: *corev1.Service")
		service := obj.(*corev1.Service)
		log.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
		log.Infof("    Service Name: %s", service.Name)
		log.Infof("    Namespace: %s", service.Namespace)

	case *corev1.Endpoints:
		log.Infoln("ControllerHandler ObjectCreated with type: *corev1.Endpoints")
		endpoints := obj.(*corev1.Endpoints)
		log.Infof("    ResourceVersion: %s", endpoints.ObjectMeta.ResourceVersion)
		log.Infof("    Endpoints Name: %s", endpoints.Name)
		log.Infof("    Namespace: %s", endpoints.Namespace)

	case *corev1.Namespace:
		log.Infoln("ControllerHandler ObjectCreated with type: *corev1.Namespace")
		namespace := obj.(*corev1.Namespace)
		log.Infof("    ResourceVersion: %s", namespace.ObjectMeta.ResourceVersion)
		log.Infof("    Namespace Name: %s", namespace.Name)

	}
}

// ObjectDeleted is called when an object is deleted
func (t *ControllerHandler) ObjectDeleted(obj interface{}) {
	log.Info("TestHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (t *ControllerHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Info("TestHandler.ObjectUpdated")
}
