package controller

import (
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/message"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Handler is an implementation of a ResourceEventHandler.
type Handler struct {
	ignored      k8s.IgnoreWrapper
	messageQueue workqueue.RateLimitingInterface
}

func NewHandler(ignored k8s.IgnoreWrapper, messageQueue workqueue.RateLimitingInterface) *Handler {
	h := &Handler{
		ignored:      ignored,
		messageQueue: messageQueue,
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

func (h *Handler) OnAdd(obj interface{}) {
	// convert the resource object into a key (in this case
	// we are just doing it in the format of 'namespace/name')
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	// add the key to the queue for the handler to get
	// If object key is not in our list of ignored namespaces
	if !k8s.ObjectKeyInNamespace(key, h.ignored.Namespaces) {
		event := message.Message{
			Key:    key,
			Object: obj,
			Action: message.TypeCreated,
		}
		h.messageQueue.Add(event)
	}
}

func (h *Handler) OnUpdate(oldObj, newObj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(newObj)
	if err != nil {
		return
	}

	if !k8s.ObjectKeyInNamespace(key, h.ignored.Namespaces) {
		event := message.Message{
			Key:       key,
			Object:    newObj,
			OldObject: oldObj,
			Action:    message.TypeUpdated,
		}
		h.messageQueue.Add(event)
	}
}

func (h *Handler) OnDelete(obj interface{}) {
	// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
	// us to check the DeletedFinalStateUnknown existence in the event that
	// a resource was deleted but it is still contained in the index
	//
	// this then in turn calls MetaNamespaceKeyFunc
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	if !k8s.ObjectKeyInNamespace(key, h.ignored.Namespaces) {
		event := message.Message{
			Key:    key,
			Object: obj,
			Action: message.TypeDeleted,
		}
		h.messageQueue.Add(event)
	}
}
