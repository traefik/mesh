package message

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/containous/traefik/pkg/config"
)

const (
	TypeCreated = "created"
	TypeUpdated = "updated"
	TypeDeleted = "deleted"

	ConfigServiceVersionKey string = "i3o-config-service-version-key"
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

// GetVersion gets the version of a deploy message.
func (d *Deploy) GetVersion() (time.Time, error) {
	if d.Config.HTTP != nil {
		if value, exists := d.Config.HTTP.Services[ConfigServiceVersionKey]; exists {
			nano, err := strconv.ParseInt(value.LoadBalancer.Servers[0].URL, 10, 64)
			return time.Unix(0, nano), err
		}
	}
	return time.Now(), errors.New("Could not parse version from Deploy")
}

// Config holds a message for processing in the config queue.
type Config struct {
	Config *config.Configuration
}

func BuildNewConfigWithVersion(conf *config.Configuration) Config {
	t := time.Now().UnixNano()
	c := conf.DeepCopy()
	c.HTTP.Services[ConfigServiceVersionKey] = &config.Service{
		LoadBalancer: &config.LoadBalancerService{
			Servers: []config.Server{
				{
					URL: fmt.Sprintf("%d", t),
				},
			},
		},
	}
	return Config{
		Config: c,
	}
}

// GetVersion gets the version of a config message.
func (c *Config) GetVersion() (time.Time, error) {
	if c.Config.HTTP != nil {
		if value, exists := c.Config.HTTP.Services[ConfigServiceVersionKey]; exists {
			nano, err := strconv.ParseInt(value.LoadBalancer.Servers[0].URL, 10, 64)
			return time.Unix(0, nano), err
		}
	}
	return time.Now(), errors.New("Could not parse version from Config")
}
