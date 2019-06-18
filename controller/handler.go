package controller

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(event ControllerMessage)
	ObjectDeleted(event ControllerMessage)
	ObjectUpdated(event ControllerMessage)
}
