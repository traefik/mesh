package cleanup

import (
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Cleaner is an interface for the cleanup methods.
type Cleaner interface {
	CleanShadowServices() error
}

// Ensure the Prepare fits the Preparer interface
var _ Cleaner = (*Cleanup)(nil)

// Cleanup holds the clients for the various resource controllers.
type Cleanup struct {
	client k8s.Client
	log    logrus.FieldLogger
}

// NewCleanup returns an initialized cleanup object.
func NewCleanup(log logrus.FieldLogger, client k8s.Client) Cleaner {
	return &Cleanup{
		client: client,
		log:    log,
	}
}

// CleanShadowServices deletes all shadow services from the cluster.
func (c *Cleanup) CleanShadowServices() error {
	serviceList, err := c.client.GetKubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{
		LabelSelector: "app=maesh",
	})
	if len(serviceList.Items) == 0 {
		// No services found, return without error.
		return nil
	}

	if err != nil {
		return err
	}

	for _, s := range serviceList.Items {
		if err := c.client.GetKubernetesClient().CoreV1().Services(s.Namespace).Delete(s.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}
