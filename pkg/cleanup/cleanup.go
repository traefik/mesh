package cleanup

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/pkg/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Cleanup holds the clients for the various resource controllers.
type Cleanup struct {
	namespace  string
	kubeClient kubernetes.Interface
	dnsClient  *dns.Client
	logger     logrus.FieldLogger
}

// NewCleanup returns an initialized cleanup object.
func NewCleanup(logger logrus.FieldLogger, kubeClient kubernetes.Interface, namespace string) *Cleanup {
	dnsClient := dns.NewClient(logger, kubeClient)

	return &Cleanup{
		kubeClient: kubeClient,
		logger:     logger,
		namespace:  namespace,
		dnsClient:  dnsClient,
	}
}

// CleanShadowServices deletes all shadow services from the cluster.
func (c *Cleanup) CleanShadowServices(ctx context.Context) error {
	serviceList, err := c.kubeClient.CoreV1().Services(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
	if err != nil {
		return err
	}

	for _, s := range serviceList.Items {
		if err := c.kubeClient.CoreV1().Services(s.Namespace).Delete(ctx, s.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// RestoreDNSConfig restores the configmap and restarts the DNS pods.
func (c *Cleanup) RestoreDNSConfig(ctx context.Context) error {
	provider, err := c.dnsClient.CheckDNSProvider(ctx)
	if err != nil {
		return err
	}

	// Restore configmaps based on DNS provider.
	switch provider {
	case dns.CoreDNS:
		if err := c.dnsClient.RestoreCoreDNS(ctx); err != nil {
			return fmt.Errorf("unable to restore CoreDNS: %w", err)
		}
	case dns.KubeDNS:
		if err := c.dnsClient.RestoreKubeDNS(ctx); err != nil {
			return fmt.Errorf("unable to restore KubeDNS: %w", err)
		}
	}

	return nil
}
