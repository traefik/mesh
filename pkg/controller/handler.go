package controller

import (
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Handler is an implementation of a ResourceEventHandler.
type Handler struct {
	log               logrus.FieldLogger
	ignored           k8s.IgnoreWrapper
	configRefreshChan chan string
	serviceManager    ServiceManager
}

// NewHandler creates a handler.
func NewHandler(log logrus.FieldLogger, ignored k8s.IgnoreWrapper, serviceManager ServiceManager, configRefreshChan chan string) *Handler {
	h := &Handler{
		log:               log,
		ignored:           ignored,
		configRefreshChan: configRefreshChan,
		serviceManager:    serviceManager,
	}

	if err := h.Init(); err != nil {
		log.Errorln("Could not initialize MeshControllerHandler")
	}

	return h
}

// Init handles any handler initialization.
func (h *Handler) Init() error {
	h.log.Debugln("MeshControllerHandler.Init")

	return nil
}

// OnAdd executed when an object is added.
func (h *Handler) OnAdd(obj interface{}) {
	// assert the type to an object to pull out relevant data
	switch obj := obj.(type) {
	case *corev1.Service:
		if h.ignored.IsIgnored(obj.ObjectMeta) {
			return
		}

		if err := h.serviceManager.Create(obj); err != nil {
			h.log.Errorf("Could not create mesh service: %v", err)
		}
	case *corev1.Endpoints:
		return
	case *corev1.Pod:
		if !isMeshPod(obj) {
			return
		}
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- k8s.ConfigMessageChanRebuild
}

// OnUpdate executed when an object is updated.
func (h *Handler) OnUpdate(oldObj, newObj interface{}) {
	// Assert the type to an object to pull out relevant data.
	switch obj := newObj.(type) {
	case *corev1.Service:
		if h.ignored.IsIgnored(obj.ObjectMeta) {
			return
		}

		oldSvc, ok := oldObj.(*corev1.Service)
		if !ok {
			h.log.Errorf("Old object is not a kubernetes Service")
			return
		}

		if _, err := h.serviceManager.Update(oldSvc, obj); err != nil {
			h.log.Errorf("Could not update mesh service: %v", err)
		}

		h.log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)
	case *corev1.Endpoints:
		// We can use the same ignore for services and endpoints.
		if h.ignored.IsIgnored(obj.ObjectMeta) {
			return
		}

		h.log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)
	case *corev1.Pod:
		if !isMeshPod(obj) {
			// We don't track updates of user pods, updates are done through endpoints.
			return
		}

		h.log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Pod: %s/%s", obj.Namespace, obj.Name)
		// Since this is a mesh pod update, trigger a force deploy.
		h.configRefreshChan <- k8s.ConfigMessageChanForce

		return
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- k8s.ConfigMessageChanRebuild
}

// OnDelete executed when an object is deleted.
func (h *Handler) OnDelete(obj interface{}) {
	// Assert the type to an object to pull out relevant data.
	switch obj := obj.(type) {
	case *corev1.Service:
		if h.ignored.IsIgnored(obj.ObjectMeta) {
			return
		}

		h.log.Debugf("MeshControllerHandler ObjectDeleted with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		if err := h.serviceManager.Delete(obj); err != nil {
			h.log.Errorf("Could not delete mesh service: %v", err)
		}
	case *corev1.Endpoints:
		// We can use the same ignore for services and endpoints.
		if h.ignored.IsIgnored(obj.ObjectMeta) {
			return
		}

		h.log.Debugf("MeshController ObjectDeleted with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)
	case *corev1.Pod:
		return
	}

	// Trigger a configuration rebuild.
	h.configRefreshChan <- k8s.ConfigMessageChanRebuild
}
