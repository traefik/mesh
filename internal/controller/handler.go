package controller

import (
	"github.com/containous/maesh/internal/k8s"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Handler is an implementation of a ResourceEventHandler.
type Handler struct {
	ignored               k8s.IgnoreWrapper
	configRefreshChan     chan bool
	createMeshServiceFunc func(service *corev1.Service) error
	updateMeshServiceFunc func(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error)
	deleteMeshServiceFunc func(serviceName, serviceNamespace string) error
}

// NewHandler creates a handler.
func NewHandler(ignored k8s.IgnoreWrapper, configRefreshChan chan bool) *Handler {
	h := &Handler{
		ignored:           ignored,
		configRefreshChan: configRefreshChan,
	}

	if err := h.Init(); err != nil {
		log.Errorln("Could not initialize MeshControllerHandler")
	}

	return h
}

// Init handles any handler initialization.
func (h *Handler) Init() error {
	log.Debugln("MeshControllerHandler.Init")

	return nil
}

// RegisterMeshHandlers registers function handlers.
func (h *Handler) RegisterMeshHandlers(createFunc func(service *corev1.Service) error, updateFunc func(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error), deleteFunc func(serviceName, serviceNamespace string) error) {
	h.createMeshServiceFunc = createFunc
	h.updateMeshServiceFunc = updateFunc
	h.deleteMeshServiceFunc = deleteFunc
}

// OnAdd executed when an object is added.
func (h *Handler) OnAdd(obj interface{}) {
	// assert the type to an object to pull out relevant data
	switch obj := obj.(type) {
	case *corev1.Service:
		if h.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		if err := h.createMeshServiceFunc(obj); err != nil {
			log.Errorf("Could not create mesh service: %v", err)
		}
	case *corev1.Endpoints:
		return
	case *corev1.Pod:
		if !isMeshPod(obj) {
			return
		}
	}

	// Trigger a configuration refresh.
	h.configRefreshChan <- true
}

// OnUpdate executed when an object is updated.
func (h *Handler) OnUpdate(oldObj, newObj interface{}) {
	// assert the type to an object to pull out relevant data
	switch obj := newObj.(type) {
	case *corev1.Service:
		if h.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		oldService := oldObj.(*corev1.Service)
		if _, err := h.updateMeshServiceFunc(oldService, obj); err != nil {
			log.Errorf("Could not update mesh service: %v", err)
		}

		log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)
	case *corev1.Endpoints:
		if h.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)
	case *corev1.Pod:
		if !isMeshPod(obj) {
			return
		}

		log.Debugf("MeshControllerHandler ObjectUpdated with type: *corev1.Pod: %s/%s", obj.Namespace, obj.Name)
	}

	// Trigger a configuration refresh.
	h.configRefreshChan <- true
}

// OnDelete executed when an object is deleted.
func (h *Handler) OnDelete(obj interface{}) {
	// assert the type to an object to pull out relevant data
	switch obj := obj.(type) {
	case *corev1.Service:
		if h.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshControllerHandler ObjectDeleted with type: *corev1.Service: %s/%s", obj.Namespace, obj.Name)

		if err := h.deleteMeshServiceFunc(obj.Name, obj.Namespace); err != nil {
			log.Errorf("Could not delete mesh service: %v", err)
		}
	case *corev1.Endpoints:
		if h.ignored.Ignored(obj.Name, obj.Namespace) {
			return
		}

		log.Debugf("MeshController ObjectDeleted with type: *corev1.Endpoints: %s/%s", obj.Namespace, obj.Name)
	case *corev1.Pod:
		return
	}

	// Trigger a configuration refresh.
	h.configRefreshChan <- true
}
