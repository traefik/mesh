package cleanup

import (
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Cleanup holds the clients for the various resource controllers.
type Cleanup struct {
	client    k8s.Client
	log       logrus.FieldLogger
	namespace string
}

// NewCleanup returns an initialized cleanup object.
func NewCleanup(log logrus.FieldLogger, client k8s.Client, namespace string) *Cleanup {
	return &Cleanup{
		client:    client,
		log:       log,
		namespace: namespace,
	}
}

// CleanShadowServices deletes all shadow services from the cluster.
func (c *Cleanup) CleanShadowServices() error {
	serviceList, err := c.client.GetKubernetesClient().CoreV1().Services(c.namespace).List(metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
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
