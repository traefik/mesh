package prepare

import (
	"context"
	"fmt"

	"github.com/traefik/mesh/v2/cmd"
	"github.com/traefik/mesh/v2/pkg/dns"
	"github.com/traefik/mesh/v2/pkg/k8s"
	"github.com/traefik/paerser/cli"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCmd builds a new Prepare command.
func NewCmd(config *Configuration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "prepare",
		Description:   `Prepare command.`,
		Configuration: config,
		Run: func(_ []string) error {
			return prepareCommand(config)
		},
		Resources: loaders,
	}
}

func prepareCommand(config *Configuration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	log, err := cmd.NewLogger(config.LogFormat, config.LogLevel)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	log.Debug("Starting prepare...")
	log.Debugf("Using masterURL: %q", config.MasterURL)
	log.Debugf("Using kubeconfig: %q", config.KubeConfig)

	client, err := k8s.NewClient(log, config.MasterURL, config.KubeConfig)
	if err != nil {
		return fmt.Errorf("unable to create kubernetes client: %w", err)
	}

	dnsClient := dns.NewClient(log, client.KubernetesClient())

	log.Debugf("ACL mode enabled: %t", config.ACL)

	if err = k8s.CheckSMIVersion(client.KubernetesClient(), config.ACL); err != nil {
		return fmt.Errorf("unsupported SMI version: %w", err)
	}

	var dnsProvider dns.Provider

	dnsProvider, err = dnsClient.CheckDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("unable to find suitable DNS provider: %w", err)
	}

	switch dnsProvider {
	case dns.CoreDNS:
		if err := dnsClient.ConfigureCoreDNS(ctx, metav1.NamespaceSystem, config.ClusterDomain, config.Namespace); err != nil {
			return fmt.Errorf("unable to configure CoreDNS: %w", err)
		}
	case dns.KubeDNS:
		if err := dnsClient.ConfigureKubeDNS(ctx, config.ClusterDomain, config.Namespace); err != nil {
			return fmt.Errorf("unable to configure KubeDNS: %w", err)
		}
	}

	return nil
}
