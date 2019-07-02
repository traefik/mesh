package deployer

import (
	"github.com/containous/traefik/pkg/config"
)

// Message holds a message type for processing in the controller queue.
type Message struct {
	PodName         string
	PodIP           string
	ConfigmapDeploy bool
	Config          *config.Configuration
}
