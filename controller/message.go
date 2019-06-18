package controller

const (
	MessageTypeCreated = "created"
	MessageTypeUpdated = "updated"
	MessageTypeDeleted = "deleted"
)

// ControllerMessage holds a message type for processing in the controller queue.
type ControllerMessage struct {
	Key       string
	Object    interface{}
	OldObject interface{}
	Action    string
}
