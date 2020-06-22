package cleanup

import (
	"fmt"

	"github.com/containous/maesh/pkg/dns"
	"github.com/sirupsen/logrus"
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
func (c *Cleanup) CleanShadowServices() error {
	serviceList, err := c.kubeClient.CoreV1().Services(c.namespace).List(metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
	if err != nil {
		return err
	}

	for _, s := range serviceList.Items {
		if err := c.kubeClient.CoreV1().Services(s.Namespace).Delete(s.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// RestoreDNSConfig restores the configmap and restarts the DNS pods.
func (c *Cleanup) RestoreDNSConfig() error {
	provider, err := c.dnsClient.CheckDNSProvider()
	if err != nil {
		return err
	}

	// Restore configmaps based on DNS provider.
	switch provider {
	case dns.CoreDNS:
		if err := c.dnsClient.RestoreCoreDNS(); err != nil {
			return fmt.Errorf("unable to restore CoreDNS: %w", err)
		}
	case dns.KubeDNS:
		if err := c.dnsClient.RestoreKubeDNS(); err != nil {
			return fmt.Errorf("unable to restore KubeDNS: %w", err)
		}
	}

	return nil
}
