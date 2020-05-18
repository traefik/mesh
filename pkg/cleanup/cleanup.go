package cleanup

import (
	"fmt"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/prepare"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Cleanup holds the clients for the various resource controllers.
type Cleanup struct {
	client    k8s.Client
	log       logrus.FieldLogger
	namespace string
	prep      *prepare.Prepare
}

// NewCleanup returns an initialized cleanup object.
func NewCleanup(log logrus.FieldLogger, client k8s.Client, namespace string) *Cleanup {
	p := prepare.NewPrepare(log, client)

	return &Cleanup{
		client:    client,
		log:       log,
		namespace: namespace,
		prep:      p,
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

// RestoreDNSConfig restores the backed up configmap, restarts the DNS pods, then deletes the backup.
func (c *Cleanup) RestoreDNSConfig() error {
	var (
		deployment      *appsv1.Deployment
		configmapBackup *corev1.ConfigMap
		err             error
	)

	provider, err := c.prep.CheckDNSProvider()
	if err != nil {
		return err
	}

	// Restore backup based on DNS provider.
	switch provider {
	case prepare.CoreDNS:
		deployment, configmapBackup, err = c.restoreCoreDNS()
		if err != nil {
			return fmt.Errorf("unable to restore CoreDNS: %w", err)
		}
	case prepare.KubeDNS:
		deployment, configmapBackup, err = c.restoreKubeDNS()
		if err != nil {
			return fmt.Errorf("unable to restore KubeDNS: %w", err)
		}
	}

	// Restart the DNS pods
	if err = c.prep.RestartPods(deployment); err != nil {
		return err
	}

	// Delete backup configmap.
	if err := c.client.GetKubernetesClient().CoreV1().ConfigMaps(configmapBackup.Namespace).Delete(configmapBackup.Name, &metav1.DeleteOptions{}); err != nil {
		return err
	}

	return nil
}

func (c *Cleanup) restoreCoreDNS() (*appsv1.Deployment, *corev1.ConfigMap, error) {
	deployment, err := c.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	coreConfigMap, err := c.prep.GetCorefileConfigMap(deployment)
	if err != nil {
		return nil, nil, err
	}

	return deployment, coreConfigMap, nil
}

func (c *Cleanup) restoreKubeDNS() (*appsv1.Deployment, *corev1.ConfigMap, error) {
	deployment, err := c.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	kubeConfigMap, err := c.prep.GetKubeDNSConfigMap(deployment)
	if err != nil {
		return nil, nil, err
	}

	return deployment, kubeConfigMap, nil
}
