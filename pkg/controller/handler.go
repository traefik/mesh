package controller

import (
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Handler is an implementation of a ResourceEventHandler.
type Handler struct {
	log               logrus.FieldLogger
	configRefreshChan chan struct{}
	serviceManager    ServiceManager
}

// NewHandler creates a handler.
func NewHandler(log logrus.FieldLogger, serviceManager ServiceManager, configRefreshChan chan struct{}) *Handler {
	return &Handler{
		log:               log,
		configRefreshChan: configRefreshChan,
		serviceManager:    serviceManager,
	}
}

// OnAdd is called when an object is added.
func (h *Handler) OnAdd(obj interface{}) {
	// If the created object is a service we have to create a corresponding shadow service.
	if obj, isService := obj.(*corev1.Service); isService {
		if err := h.serviceManager.Create(obj); err != nil {
			h.log.Errorf("Could not create mesh service: %v", err)
		}
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- struct{}{}
}

// OnUpdate is called when an object is updated.
func (h *Handler) OnUpdate(oldObj, newObj interface{}) {
	// If the updated object is a service we have to update the corresponding shadow service.
	if obj, isService := newObj.(*corev1.Service); isService {
		oldSvc, ok := oldObj.(*corev1.Service)
		if !ok {
			h.log.Errorf("Old object is not a kubernetes Service")
			return
		}

		if _, err := h.serviceManager.Update(oldSvc, obj); err != nil {
			h.log.Errorf("Could not update mesh service: %v", err)
		}

		h.log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- struct{}{}
}

// OnDelete is called when an object is deleted.
func (h *Handler) OnDelete(obj interface{}) {
	// If the deleted object is a service we have to delete the corresponding shadow service.
	if obj, isService := obj.(*corev1.Service); isService {
		h.log.Debugf("MeshControllerHandler ObjectDeleted with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		if err := h.serviceManager.Delete(obj); err != nil {
			h.log.Errorf("Could not delete mesh service: %v", err)
		}
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- struct{}{}
}
