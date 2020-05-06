package controller

import (
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// configMessageChanRebuild rebuild.
const configMessageChanRebuild = "rebuild"

// Handler is an implementation of a ResourceEventHandler.
type Handler struct {
	log               logrus.FieldLogger
	ignoredResources  k8s.IgnoreWrapper
	configRefreshChan chan string
	serviceManager    ServiceManager
}

// NewHandler creates a handler.
func NewHandler(log logrus.FieldLogger, ignored k8s.IgnoreWrapper, serviceManager ServiceManager, configRefreshChan chan string) *Handler {
	return &Handler{
		log:               log,
		ignoredResources:  ignored,
		configRefreshChan: configRefreshChan,
		serviceManager:    serviceManager,
	}
}

// OnAdd is called when an object is added.
func (h *Handler) OnAdd(obj interface{}) {
	if h.isIgnoredResource(obj) {
		return
	}

	// If the created object is a service we should create a corresponding shadow service.
	if obj, isService := obj.(*corev1.Service); isService {
		if err := h.serviceManager.Create(obj); err != nil {
			h.log.Errorf("Could not create mesh service: %v", err)
		}
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- configMessageChanRebuild
}

// OnUpdate is called when an object is updated and ensures that the proper handler is called depending on the filter matches.
func (h *Handler) OnUpdate(oldObj, newObj interface{}) {
	isOldIgnored := h.isIgnoredResource(oldObj)
	isNewIgnored := h.isIgnoredResource(newObj)

	switch {
	case isOldIgnored && isNewIgnored:
		return

	// Old object is not ignored anymore, so we can treat this as an add event.
	case isOldIgnored && !isNewIgnored:
		h.OnAdd(newObj)
		return

	// Old object is now ignored, so we can treat this as a delete event.
	case !isOldIgnored && isNewIgnored:
		h.OnDelete(oldObj)
		return
	}

	// Old and new object are not ignored so this is a real update.
	// If the updated object is a service we should update the corresponding shadow service.
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
	h.configRefreshChan <- configMessageChanRebuild
}

// OnDelete is called when an object is deleted.
func (h *Handler) OnDelete(obj interface{}) {
	if h.isIgnoredResource(obj) {
		return
	}

	// If the deleted object is a service we should delete the corresponding shadow service.
	if obj, isService := obj.(*corev1.Service); isService {
		h.log.Debugf("MeshControllerHandler ObjectDeleted with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		if err := h.serviceManager.Delete(obj); err != nil {
			h.log.Errorf("Could not delete mesh service: %v", err)
		}
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- configMessageChanRebuild
}

// isIgnoredResource returns true if the given resource should be ignored, false otherwise.
func (h *Handler) isIgnoredResource(obj interface{}) bool {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return true
	}

	pMeta := meta.AsPartialObjectMetadata(accessor)

	return h.ignoredResources.IsIgnored(pMeta.ObjectMeta)
}
