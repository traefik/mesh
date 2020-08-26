package prepare

import (
	"context"
	"fmt"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/pkg/dns"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/traefik/paerser/cli"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCmd builds a new Prepare command.
func NewCmd(pConfig *cmd.PrepareConfiguration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "prepare",
		Description:   `Prepare command.`,
		Configuration: pConfig,
		Run: func(_ []string) error {
			return prepareCommand(pConfig)
		},
		Resources: loaders,
	}
}

func prepareCommand(pConfig *cmd.PrepareConfiguration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	log, err := cmd.NewLogger(pConfig.LogFormat, pConfig.LogLevel, pConfig.Debug)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	log.Debug("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", pConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", pConfig.KubeConfig)

	client, err := k8s.NewClient(log, pConfig.MasterURL, pConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("unable to create kubernetes client: %w", err)
	}

	dnsClient := dns.NewClient(log, client.KubernetesClient())

	if pConfig.SMI {
		log.Warnf("SMI mode is deprecated, please consider using --acl instead")
	}

	aclEnabled := pConfig.ACL || pConfig.SMI

	log.Debugf("ACL mode enabled: %t", aclEnabled)

	if err = k8s.CheckSMIVersion(client.KubernetesClient(), aclEnabled); err != nil {
		return fmt.Errorf("unsupported SMI version: %w", err)
	}

	var dnsProvider dns.Provider

	dnsProvider, err = dnsClient.CheckDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("unable to find suitable DNS provider: %w", err)
	}

	switch dnsProvider {
	case dns.CoreDNS:
		if err := dnsClient.ConfigureCoreDNS(ctx, metav1.NamespaceSystem, pConfig.ClusterDomain, pConfig.Namespace); err != nil {
			return fmt.Errorf("unable to configure CoreDNS: %w", err)
		}
	case dns.KubeDNS:
		if err := dnsClient.ConfigureKubeDNS(ctx, pConfig.ClusterDomain, pConfig.Namespace); err != nil {
			return fmt.Errorf("unable to configure KubeDNS: %w", err)
		}
	}

	return nil
}
