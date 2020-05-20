package cleanup

import (
	"fmt"

	"github.com/containous/maesh/pkg/dns"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Cleanup holds the clients for the various resource controllers.
type Cleanup struct {
	client    k8s.Client
	log       logrus.FieldLogger
	namespace string
	dns       *dns.Client
}

// NewCleanup returns an initialized cleanup object.
func NewCleanup(log logrus.FieldLogger, client k8s.Client, namespace string) *Cleanup {
	dns := dns.NewClient(log, client)

	return &Cleanup{
		client:    client,
		log:       log,
		namespace: namespace,
		dns:       dns,
	}
}

// CleanShadowServices deletes all shadow services from the cluster.
func (c *Cleanup) CleanShadowServices() error {
	serviceList, err := c.client.KubernetesClient().CoreV1().Services(c.namespace).List(metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
	if err != nil {
		return err
	}

	for _, s := range serviceList.Items {
		if err := c.client.KubernetesClient().CoreV1().Services(s.Namespace).Delete(s.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// RestoreDNSConfig restores the configmap and restarts the DNS pods.
func (c *Cleanup) RestoreDNSConfig() error {
	provider, err := c.dns.CheckDNSProvider()
	if err != nil {
		return err
	}

	// Restore configmaps based on DNS provider.
	switch provider {
	case dns.CoreDNS:
		if err := c.dns.RestoreCoreDNS(); err != nil {
			return fmt.Errorf("unable to restore CoreDNS: %w", err)
		}
	case dns.KubeDNS:
		if err := c.dns.RestoreKubeDNS(); err != nil {
			return fmt.Errorf("unable to restore KubeDNS: %w", err)
		}
	}

	return nil
}
