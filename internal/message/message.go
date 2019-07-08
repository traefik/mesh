package message

import (
	"github.com/containous/traefik/pkg/config"
)

const (
	TypeCreated = "created"
	TypeUpdated = "updated"
	TypeDeleted = "deleted"
)

// Message holds a message for processing in the message queue.
type Message struct {
	Key       string
	Object    interface{}
	OldObject interface{}
	Action    string
}

// Deploy holds a message for processing in the deploy queue.
type Deploy struct {
	PodName         string
	PodIP           string
	ConfigmapDeploy bool
	Config          *config.Configuration
}

// Config holds a message for processing in the config queue.
type Config struct {
	Config *config.Configuration
}
