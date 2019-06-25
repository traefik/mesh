package smi

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/containous/traefik/pkg/config"
	"github.com/containous/traefik/pkg/job"
	"github.com/containous/traefik/pkg/log"
	"github.com/containous/traefik/pkg/safe"
)

// Provider holds configurations of the provider.
type Provider struct {
	Endpoint          string   `description:"Kubernetes server endpoint (required for external cluster client)."`
	Token             string   `description:"Kubernetes bearer token (not needed for in-cluster client)."`
	CertAuthFilePath  string   `description:"Kubernetes certificate authority file path (not needed for in-cluster client)."`
	Namespaces        []string `description:"Kubernetes namespaces." export:"true"`
	lastConfiguration safe.Safe
}

func (p *Provider) newK8sClient(ctx context.Context) (*clientWrapper, error) {
	logger := log.FromContext(ctx)

	withEndpoint := ""
	if p.Endpoint != "" {
		withEndpoint = fmt.Sprintf(" with endpoint %v", p.Endpoint)
	}

	var cl *clientWrapper
	var err error
	switch {
	case os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "":
		logger.Infof("Creating in-cluster Provider client%s", withEndpoint)
		cl, err = newInClusterClient(p.Endpoint)
	case os.Getenv("KUBECONFIG") != "":
		logger.Infof("Creating cluster-external Provider client from KUBECONFIG %s", os.Getenv("KUBECONFIG"))
		cl, err = newExternalClusterClientFromFile(os.Getenv("KUBECONFIG"))
	default:
		logger.Infof("Creating cluster-external Provider client%s", withEndpoint)
		cl, err = newExternalClusterClient(p.Endpoint, p.Token, p.CertAuthFilePath)
	}

	return cl, err
}

// Init the provider.
func (p *Provider) Init() error {
	return nil
}

// Provide allows the k8s provider to provide configurations to traefik
// using the given configuration channel.
func (p *Provider) Provide(configurationChan chan<- config.Message, pool *safe.Pool) error {
	ctxLog := log.With(context.Background(), log.Str(log.ProviderName, "kubernetes"))
	logger := log.FromContext(ctxLog)
	//// Tell glog (used by client-go) to log into STDERR. Otherwise, we risk
	//// certain kinds of API errors getting logged into a directory not
	//// available in a `FROM scratch` Docker container, causing glog to abort
	//// hard with an exit code > 0.
	//err := flag.Set("logtostderr", "true")
	//if err != nil {
	//	return err
	//}

	k8sClient, err := p.newK8sClient(ctxLog)
	if err != nil {
		return err
	}

	pool.Go(func(stop chan bool) {
		operation := func() error {
			stopWatch := make(chan struct{}, 1)
			defer close(stopWatch)

			eventsChan, err := k8sClient.WatchAll(p.Namespaces, stopWatch)
			if err != nil {
				logger.Errorf("Error watching kubernetes events: %v", err)
				timer := time.NewTimer(1 * time.Second)
				select {
				case <-timer.C:
					return err
				case <-stop:
					return nil
				}
			}

			for {
				select {
				case <-stop:
					return nil
				case event := <-eventsChan:
					conf := p.loadConfigurationFromSMI(ctxLog, k8sClient)

					if reflect.DeepEqual(p.lastConfiguration.Get(), conf) {
						logger.Debugf("Skipping Kubernetes event kind %T", event)
					} else {
						p.lastConfiguration.Set(conf)
						configurationChan <- config.Message{
							ProviderName:  "kubernetes",
							Configuration: conf,
						}
					}
				}
			}
		}

		notify := func(err error, time time.Duration) {
			logger.Errorf("Provider connection error: %s; retrying in %s", err, time)
		}
		err := backoff.RetryNotify(safe.OperationWithRecover(operation), job.NewBackOff(backoff.NewExponentialBackOff()), notify)
		if err != nil {
			logger.Errorf("Cannot connect to Provider: %s", err)
		}
	})

	return nil
}

func (p *Provider) loadConfigurationFromSMI(_ context.Context, _ Client) *config.Configuration {
	conf := &config.Configuration{
		HTTP: &config.HTTPConfiguration{
			Routers:     map[string]*config.Router{},
			Middlewares: map[string]*config.Middleware{},
			Services:    map[string]*config.Service{},
		},
		TCP: &config.TCPConfiguration{},
	}

	return conf
}
