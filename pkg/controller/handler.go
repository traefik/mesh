package controller

import (
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type enqueueWorkHandler struct {
	logger    logrus.FieldLogger
	workQueue workqueue.RateLimitingInterface
}

// OnAdd is called when an object is added to the informers cache.
func (h *enqueueWorkHandler) OnAdd(obj interface{}) {
	h.enqueueWork(obj)
}

// OnUpdate is called when an object is updated in the informers cache.
func (h *enqueueWorkHandler) OnUpdate(oldObj interface{}, newObj interface{}) {
	oldObjMeta, okOld := oldObj.(metav1.Object)
	newObjMeta, okNew := newObj.(metav1.Object)

	// This is a resync event, no extra work is needed.
	if okOld && okNew && oldObjMeta.GetResourceVersion() == newObjMeta.GetResourceVersion() {
		return
	}

	h.enqueueWork(newObj)
}

// OnDelete is called when an object is removed from the informers cache.
func (h *enqueueWorkHandler) OnDelete(obj interface{}) {
	h.enqueueWork(obj)
}

func (h *enqueueWorkHandler) enqueueWork(obj interface{}) {
	if _, isService := obj.(*corev1.Service); !isService {
		h.workQueue.Add(configRefreshKey)
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		h.logger.Errorf("Unable to create a work key for resource %#v", obj)
		return
	}

	h.workQueue.Add(key)
}
