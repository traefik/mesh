package static

import (
	httpProvider "github.com/containous/maesh/pkg/proxy/provider/http"
	traefikStatic "github.com/containous/traefik/v2/pkg/config/static"
	"github.com/containous/traefik/v2/pkg/ping"
	"github.com/containous/traefik/v2/pkg/provider/file"
	"github.com/containous/traefik/v2/pkg/provider/rest"
	"github.com/containous/traefik/v2/pkg/types"
)

// DefaultInternalEntryPointName the name of the default internal entry point
const DefaultInternalEntryPointName = "traefik"

// Configuration is the static configuration
type Configuration struct {
	Global *traefikStatic.Global `description:"Global configuration options" json:"global,omitempty" toml:"global,omitempty" yaml:"global,omitempty" export:"true"`

	ServersTransport *traefikStatic.ServersTransport `description:"Servers default transport." json:"serversTransport,omitempty" toml:"serversTransport,omitempty" yaml:"serversTransport,omitempty" export:"true"`
	EntryPoints      traefikStatic.EntryPoints       `description:"Entry points definition." json:"entryPoints,omitempty" toml:"entryPoints,omitempty" yaml:"entryPoints,omitempty" export:"true"`
	Providers        *Providers                      `description:"Providers configuration." json:"providers,omitempty" toml:"providers,omitempty" yaml:"providers,omitempty" export:"true"`

	API     *traefikStatic.API `description:"Enable api/dashboard." json:"api,omitempty" toml:"api,omitempty" yaml:"api,omitempty" label:"allowEmpty" export:"true"`
	Metrics *types.Metrics     `description:"Enable a metrics exporter." json:"metrics,omitempty" toml:"metrics,omitempty" yaml:"metrics,omitempty" export:"true"`
	Ping    *ping.Handler      `description:"Enable ping." json:"ping,omitempty" toml:"ping,omitempty" yaml:"ping,omitempty" label:"allowEmpty" export:"true"`

	Log       *types.TraefikLog      `description:"Traefik log settings." json:"log,omitempty" toml:"log,omitempty" yaml:"log,omitempty" label:"allowEmpty" export:"true"`
	AccessLog *types.AccessLog       `description:"Access log settings." json:"accessLog,omitempty" toml:"accessLog,omitempty" yaml:"accessLog,omitempty" label:"allowEmpty" export:"true"`
	Tracing   *traefikStatic.Tracing `description:"OpenTracing configuration." json:"tracing,omitempty" toml:"tracing,omitempty" yaml:"tracing,omitempty" label:"allowEmpty" export:"true"`

	HostResolver *types.HostResolverConfig `description:"Enable CNAME Flattening." json:"hostResolver,omitempty" toml:"hostResolver,omitempty" yaml:"hostResolver,omitempty" label:"allowEmpty" export:"true"`

	CertificatesResolvers map[string]traefikStatic.CertificateResolver `description:"Certificates resolvers configuration." json:"certificatesResolvers,omitempty" toml:"certificatesResolvers,omitempty" yaml:"certificatesResolvers,omitempty" export:"true"`
}

// Providers contains providers configuration
type Providers struct {
	ProvidersThrottleDuration types.Duration `description:"Backends throttle duration: minimum duration between 2 events from providers before applying a new configuration. It avoids unnecessary reloads if multiples events are sent in a short amount of time." json:"providersThrottleDuration,omitempty" toml:"providersThrottleDuration,omitempty" yaml:"providersThrottleDuration,omitempty" export:"true"`

	File *file.Provider         `description:"Enable File backend with default settings." json:"file,omitempty" toml:"file,omitempty" yaml:"file,omitempty" export:"true"`
	Rest *rest.Provider         `description:"Enable Rest backend with default settings." json:"rest,omitempty" toml:"rest,omitempty" yaml:"rest,omitempty" export:"true" label:"allowEmpty"`
	HTTP *httpProvider.Provider `description:"Enable HTTP backend with default settings." json:"http,omitempty" toml:"http,omitempty" yaml:"http,omitempty" export:"true" label:"allowEmpty"`
}

// SetEffectiveConfiguration adds missing configuration parameters derived from existing ones.
// It also takes care of maintaining backwards compatibility.
func (c *Configuration) SetEffectiveConfiguration() {
	// Creates the default entry point if needed
	if len(c.EntryPoints) == 0 {
		ep := &traefikStatic.EntryPoint{Address: ":80"}
		ep.SetDefaults()

		c.EntryPoints = traefikStatic.EntryPoints{"http": ep}
	}

	// Creates the internal traefik entry point if needed
	if (c.API != nil && c.API.Insecure) ||
		(c.Ping != nil && !c.Ping.ManualRouting && c.Ping.EntryPoint == DefaultInternalEntryPointName) ||
		(c.Metrics != nil && c.Metrics.Prometheus != nil && !c.Metrics.Prometheus.ManualRouting && c.Metrics.Prometheus.EntryPoint == DefaultInternalEntryPointName) ||
		(c.Providers != nil && c.Providers.Rest != nil && c.Providers.Rest.Insecure) {
		if _, ok := c.EntryPoints[DefaultInternalEntryPointName]; !ok {
			ep := &traefikStatic.EntryPoint{Address: ":8080"}
			ep.SetDefaults()
			c.EntryPoints[DefaultInternalEntryPointName] = ep
		}
	}
}

// ToTraefikConfig returns a Traefik compatable configuration for use in the proxy code without affecting compatibility.
func (c *Configuration) ToTraefikConfig() *traefikStatic.Configuration {
	return &traefikStatic.Configuration{
		Global:           c.Global,
		ServersTransport: c.ServersTransport,
		EntryPoints:      c.EntryPoints,
		Providers: &traefikStatic.Providers{
			ProvidersThrottleDuration: c.Providers.ProvidersThrottleDuration,
			// These are the two providers that we would provide configuration for at this time.
			File: c.Providers.File,
			Rest: c.Providers.Rest,
		},
		API:                   c.API,
		Metrics:               c.Metrics,
		Ping:                  c.Ping,
		Log:                   c.Log,
		AccessLog:             c.AccessLog,
		Tracing:               c.Tracing,
		HostResolver:          c.HostResolver,
		CertificatesResolvers: c.CertificatesResolvers,
	}
}
