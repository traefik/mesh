package i3o

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(event Message)
	ObjectDeleted(event Message)
	ObjectUpdated(event Message)
}
