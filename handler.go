package main

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

// TestHandler is a sample implementation of Handler
type TestHandler struct{}

// Init handles any handler initialization
func (t *TestHandler) Init() error {
	log.Info("TestHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (t *TestHandler) ObjectCreated(obj interface{}) {
	log.Info("TestHandler.ObjectCreated")
	// assert the type to a Pod object to pull out relevant data
	pod := obj.(*corev1.Pod)
	log.Infof("    ResourceVersion: %s", pod.ObjectMeta.ResourceVersion)
	log.Infof("    NodeName: %s", pod.Spec.NodeName)
	log.Infof("    Phase: %s", pod.Status.Phase)
}

// ObjectDeleted is called when an object is deleted
func (t *TestHandler) ObjectDeleted(obj interface{}) {
	log.Info("TestHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (t *TestHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Info("TestHandler.ObjectUpdated")
}
