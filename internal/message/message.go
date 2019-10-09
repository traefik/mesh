package message

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
)

const (
	// TypeCreated created type.
	TypeCreated = "created"
	// TypeUpdated updated type.
	TypeUpdated = "updated"
	// TypeDeleted deleted type.
	TypeDeleted = "deleted"

	// ConfigServiceVersionKey config service version key name.
	ConfigServiceVersionKey string = "maesh-config-service-version-key"
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
	PodName string
	PodIP   string
	Config  *dynamic.Configuration
}

// GetVersion gets the version of a deploy message.
func (d *Deploy) GetVersion() (time.Time, error) {
	if d.Config.HTTP != nil {
		if value, exists := d.Config.HTTP.Services[ConfigServiceVersionKey]; exists {
			nano, err := strconv.ParseInt(value.LoadBalancer.Servers[0].URL, 10, 64)
			return time.Unix(0, nano), err
		}
	}

	return time.Now(), errors.New("could not parse version from Deploy")
}

// Config holds a message for processing in the config queue.
type Config struct {
	Config *dynamic.Configuration
}

// BuildNewConfigWithVersion builds new config with version.
func BuildNewConfigWithVersion(conf *dynamic.Configuration) Config {
	t := time.Now().UnixNano()
	c := conf.DeepCopy()
	c.HTTP.Services[ConfigServiceVersionKey] = &dynamic.Service{
		LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers: []dynamic.Server{
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

	return time.Now(), errors.New("could not parse version from Config")
}
