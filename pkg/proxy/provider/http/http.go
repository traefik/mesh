package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/job"
	"github.com/containous/traefik/v2/pkg/log"
	"github.com/containous/traefik/v2/pkg/provider"
	"github.com/containous/traefik/v2/pkg/safe"
)

var _ provider.Provider = (*Provider)(nil)

// providerName is the name of the provider for logging.
const providerName = "http"

// Provider is a provider.Provider implementation that queries an endpoint for a configuration.
type Provider struct {
	Endpoint     string        `description:"Load configuration from this endpoint." json:"endpoint" toml:"endpoint" yaml:"endpoint" export:"true"`
	PollInterval time.Duration `description:"Polling interval for endpoint." json:"pollInterval,omitempty" toml:"pollInterval,omitempty" yaml:"pollInterval,omitempty"`
	PollTimeout  time.Duration `description:"Polling timeout for endpoint." json:"pollTimeout,omitempty" toml:"pollTimeout,omitempty" yaml:"pollTimeout,omitempty"`
	httpClient   *http.Client
}

// New creates a new instance of the HTTP provider.
func New(endpoint string, pollInterval time.Duration, pollTimeout time.Duration) *Provider {
	return &Provider{
		Endpoint:     endpoint,
		PollInterval: pollInterval,
		PollTimeout:  pollTimeout,
	}
}

// Init the provider.
func (p *Provider) Init() error {
	if len(p.Endpoint) == 0 {
		return fmt.Errorf("a non-empty endpoint is required")
	}

	if p.PollInterval == 0 {
		p.PollInterval = 15 * time.Second
	}

	if p.PollTimeout == 0 {
		p.PollTimeout = 15 * time.Second
	}

	p.httpClient = &http.Client{Timeout: p.PollTimeout}

	return nil
}

// Provide allows the provider to provide configurations to traefik
// using the given configuration channel.
func (p *Provider) Provide(configurationChan chan<- dynamic.Message, pool *safe.Pool) error {
	pool.GoCtx(func(routineCtx context.Context) {
		ctxLog := log.With(routineCtx, log.Str(log.ProviderName, providerName))
		logger := log.FromContext(ctxLog)

		operation := func() error {
			errChan := make(chan error)
			ticker := time.NewTicker(p.PollInterval)

			pool.GoCtx(func(ctx context.Context) {
				ctx = log.With(ctx, log.Str(log.ProviderName, providerName))

				defer close(errChan)
				for {
					select {
					case <-ticker.C:
						data, err := p.getDataFromEndpoint(ctxLog)
						if err != nil {
							logger.Errorf("Failed to get config from endpoint: %v", err)
							errChan <- err
							return
						}

						configuration := &dynamic.Configuration{}

						if err := json.Unmarshal(data, configuration); err != nil {
							log.FromContext(ctx).Errorf("Error parsing configuration: %v", err)
							return
						}

						if configuration != nil {
							configurationChan <- dynamic.Message{
								ProviderName:  providerName,
								Configuration: configuration,
							}
						}

					case <-ctx.Done():
						ticker.Stop()
						return
					}
				}
			})
			if err, ok := <-errChan; ok {
				return err
			}
			// channel closed
			return nil
		}

		notify := func(err error, time time.Duration) {
			logger.Errorf("Provider connection error, retrying in %s: %v", time, err)
		}
		err := backoff.RetryNotify(safe.OperationWithRecover(operation), backoff.WithContext(job.NewBackOff(backoff.NewExponentialBackOff()), ctxLog), notify)
		if err != nil {
			logger.Errorf("Cannot connect to HTTP server: %v", err)
		}
	})

	return nil
}

// getDataFromEndpoint returns data from the configured provider endpoint.
func (p Provider) getDataFromEndpoint(ctx context.Context) ([]byte, error) {
	resp, err := p.httpClient.Get(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to get data from endpoint %q: %w", p.Endpoint, err)
	}

	if resp == nil {
		return nil, fmt.Errorf("received no data from endpoint")
	}

	defer resp.Body.Close()

	var data []byte

	if data, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-ok response code: %d", resp.StatusCode)
	}

	log.FromContext(ctx).Debugf("Successfully received data from endpoint: %q", p.Endpoint)

	return data, nil
}
