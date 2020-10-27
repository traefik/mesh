package dns

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/cmd"
	"github.com/traefik/mesh/v2/pkg/dns"
	"github.com/traefik/mesh/v2/pkg/k8s"
	"github.com/traefik/paerser/cli"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
)

// NewCmd builds a new dns command.
func NewCmd(config *Configuration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "dns",
		Description:   `DNS command.`,
		Configuration: config,
		Run: func(_ []string) error {
			return dnsCommand(config)
		},
		Resources: loaders,
	}
}

func dnsCommand(config *Configuration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	logger, err := cmd.NewLogger(config.LogFormat, config.LogLevel)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	logger.Debug("Starting DNS server...")
	logger.Debugf("Using masterURL: %q", config.MasterURL)
	logger.Debugf("Using kubeconfig: %q", config.KubeConfig)

	clients, err := k8s.NewClient(logger, config.MasterURL, config.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	// Configure DNS.
	if err = configureDNS(ctx, clients.KubernetesClient(), logger, config); err != nil {
		return err
	}

	// Start DNS server.
	serviceLister, err := newServiceLister(ctx, clients.KubernetesClient(), config)
	if err != nil {
		return err
	}

	resolver := dns.NewShadowServiceResolver("traefik.mesh", config.Namespace, serviceLister)
	server := dns.NewServer(config.Port, resolver, logger)

	errCh := make(chan error)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("DNS server has stopped unexpectedly: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err

	case <-ctx.Done():
		if stopErr := stopDNSServer(server); stopErr != nil {
			return fmt.Errorf("unable to stop DNS server: %w", stopErr)
		}
	}

	return nil
}

func configureDNS(ctx context.Context, kubeClient kubernetes.Interface, logger logrus.FieldLogger, config *Configuration) error {
	dnsClient := dns.NewClient(logger, kubeClient)

	dnsProvider, err := dnsClient.CheckDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("unable to find suitable DNS provider: %w", err)
	}

	switch dnsProvider {
	case dns.CoreDNS:
		if err := dnsClient.ConfigureCoreDNS(ctx, config.Namespace, config.ServiceName, config.ServicePort); err != nil {
			return fmt.Errorf("unable to configure CoreDNS: %w", err)
		}

	case dns.KubeDNS:
		if err := dnsClient.ConfigureKubeDNS(ctx, config.Namespace, config.ServiceName, config.ServicePort); err != nil {
			return fmt.Errorf("unable to configure KubeDNS: %w", err)
		}
	}

	return nil
}

func newServiceLister(ctx context.Context, kubeClient kubernetes.Interface, config *Configuration) (listers.ServiceLister, error) {
	kubernetesFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, k8s.ResyncPeriod, informers.WithNamespace(config.Namespace))
	serviceLister := kubernetesFactory.Core().V1().Services().Lister()

	kubernetesFactory.Start(ctx.Done())

	for t, ok := range kubernetesFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for informer caches to sync: %s", t)
		}
	}

	return serviceLister, nil
}

func stopDNSServer(dnsServer *dns.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return dnsServer.ShutdownContext(ctx)
}
