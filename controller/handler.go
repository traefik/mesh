package controller

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(obj interface{})
	ObjectDeleted(key string, obj interface{})
	ObjectUpdated(objOld, objNew interface{})
}
