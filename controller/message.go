package controller

const (
	MessageTypeCreated = "created"
	MessageTypeUpdated = "updated"
	MessageTypeDeleted = "deleted"
)

// Message holds a message type for processing in the controller queue.
type Message struct {
	Key       string
	Object    interface{}
	OldObject interface{}
	Action    string
}
