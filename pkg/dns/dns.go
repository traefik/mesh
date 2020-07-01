package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Provider represents a DNS provider.
type Provider int

// Supported DNS providers.
const (
	UnknownDNS Provider = iota
	CoreDNS
	KubeDNS

	coreFileHeader  = "#### Begin Maesh Block"
	coreFileTrailer = "#### End Maesh Block"
)

var (
	supportedCoreDNSVersions = []string{
		"1.3",
		"1.4",
		"1.5",
		"1.6",
		"1.7",
	}
)

// Client holds the client for interacting with the k8s DNS system.
type Client struct {
	kubeClient kubernetes.Interface
	logger     logrus.FieldLogger
}

// NewClient returns an initialized DNSClient object.
func NewClient(logger logrus.FieldLogger, kubeClient kubernetes.Interface) *Client {
	return &Client{
		kubeClient: kubeClient,
		logger:     logger,
	}
}

// CheckDNSProvider checks that the DNS provider that is deployed in the cluster is supported and returns it.
func (c *Client) CheckDNSProvider() (Provider, error) {
	c.logger.Info("Checking DNS provider")

	match, err := c.coreDNSMatch()
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return CoreDNS, nil
	}

	match, err = c.kubeDNSMatch()
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return KubeDNS, nil
	}

	return UnknownDNS, errors.New("no supported DNS service available for installing maesh")
}

func (c *Client) coreDNSMatch() (bool, error) {
	c.logger.Info("Checking CoreDNS")

	deployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.logger.Debugf("CoreDNS deployment does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get CoreDNS deployment in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	var version string

	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "coredns" {
			continue
		}

		sp := strings.Split(container.Image, ":")
		version = sp[len(sp)-1]
	}

	if !isCoreDNSVersionSupported(version) {
		return false, fmt.Errorf("unsupported CoreDNS version %q, (supported versions are: %s)", version, strings.Join(supportedCoreDNSVersions, ","))
	}

	c.logger.Info("CoreDNS match")

	return true, nil
}

func isCoreDNSVersionSupported(versionLine string) bool {
	for _, v := range supportedCoreDNSVersions {
		if strings.Contains(versionLine, v) {
			return true
		}
	}

	return false
}

func (c *Client) kubeDNSMatch() (bool, error) {
	c.logger.Info("Checking KubeDNS")

	_, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.logger.Debugf("KubeDNS deployment does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get KubeDNS deployment in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	c.logger.Info("KubeDNS match")

	return true, nil
}

// ConfigureCoreDNS patches the CoreDNS configuration for Maesh.
func (c *Client) ConfigureCoreDNS(coreDNSNamespace, clusterDomain, maeshNamespace string) error {
	c.logger.Debug("Patching CoreDNS")

	coreDNSDeployment, err := c.kubeClient.AppsV1().Deployments(coreDNSNamespace).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	c.logger.Debug("Patching CoreDNS ConfigMap")

	patchedConfigMap, err := c.patchCoreDNSConfig(coreDNSDeployment, clusterDomain, maeshNamespace)
	if err != nil {
		return fmt.Errorf("unable to patch coredns config: %w", err)
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(patchedConfigMap.Namespace).Update(patchedConfigMap); err != nil {
		return err
	}

	if err := c.restartPods(coreDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchCoreDNSConfig(deployment *appsv1.Deployment, clusterDomain, maeshNamespace string) (*corev1.ConfigMap, error) {
	customConfigMap, err := c.getConfigMap(deployment.Namespace, "coredns-custom")

	// For AKS the CoreDNS config have to be added to the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		customConfigMap.Data["maesh.server"] = c.addMaeshStubDomain(
			clusterDomain,
			maeshNamespace,
			"",
		)

		return customConfigMap, nil
	}

	if !kerrors.IsNotFound(err) {
		return nil, fmt.Errorf("unable to get coredns-custom configmap: %w", err)
	}

	coreDNSConfigMap, err := c.getCorefileConfigMap(deployment)
	if err != nil {
		return nil, err
	}

	coreDNSConfigMap.Data["Corefile"] = c.addMaeshStubDomain(
		clusterDomain,
		maeshNamespace,
		coreDNSConfigMap.Data["Corefile"],
	)

	return coreDNSConfigMap, nil
}

func (c *Client) addMaeshStubDomain(clusterDomain, maeshNamespace, coreDNSConfig string) string {
	stubDomainFormat := `
%[4]s
maesh:53 {
    errors
    rewrite continue {
        name regex ([a-zA-Z0-9-_]*)\.([a-zv0-9-_]*)\.maesh %[3]s-{1}-6d61657368-{2}.%[3]s.svc.%[1]s
        answer name %[3]s-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\.%[3]s\.svc\.%[2]s {1}.{2}.maesh
    }
    kubernetes %[1]s in-addr.arpa ip6.arpa {
        pods insecure
        upstream
        fallthrough in-addr.arpa ip6.arpa
    }
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
%[5]s
`

	stubDomain := fmt.Sprintf(stubDomainFormat,
		clusterDomain,
		strings.Replace(clusterDomain, ".", "\\.", -1),
		maeshNamespace,
		coreFileHeader,
		coreFileTrailer,
	)

	// CoreDNS config already contains the maesh block.
	if strings.Contains(coreDNSConfig, coreFileHeader) {
		return coreDNSConfig
	}

	return coreDNSConfig + stubDomain
}

// ConfigureKubeDNS patches the KubeDNS configuration for Maesh.
func (c *Client) ConfigureKubeDNS(clusterDomain, maeshNamespace string) error {
	c.logger.Debug("Patching KubeDNS")

	kubeDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	c.logger.Debug("Getting CoreDNS service IP")

	var coreDNSServiceIP string

	operation := func() error {
		svc, svcErr := c.kubeClient.CoreV1().Services(maeshNamespace).Get("coredns", metav1.GetOptions{})
		if svcErr != nil {
			return fmt.Errorf("unable to get coredns service in namespace %q: %w", metav1.NamespaceSystem, err)
		}

		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("coredns service in namespace %q has no clusterip", metav1.NamespaceSystem)
		}

		coreDNSServiceIP = svc.Spec.ClusterIP

		return nil
	}

	err = backoff.Retry(safe.OperationWithRecover(operation), backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Second), 12))
	if err != nil {
		return fmt.Errorf("unable to get coredns service ip in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	c.logger.Debugf("Patching KubeDNS ConfigMap with CoreDNS service IP %q", coreDNSServiceIP)

	if err := c.patchKubeDNSConfig(kubeDNSDeployment, coreDNSServiceIP); err != nil {
		return err
	}

	if err := c.ConfigureCoreDNS(maeshNamespace, clusterDomain, maeshNamespace); err != nil {
		return err
	}

	if err := c.restartPods(kubeDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchKubeDNSConfig(deployment *appsv1.Deployment, coreDNSServiceIP string) error {
	configMap, err := c.getOrCreateKubeDNSConfigMap(deployment)
	if err != nil {
		return err
	}

	stubDomains := make(map[string][]string)

	if stubDomainsStr := configMap.Data["stubDomains"]; stubDomainsStr != "" {
		if err = json.Unmarshal([]byte(stubDomainsStr), &stubDomains); err != nil {
			return fmt.Errorf("unable to unmarshal stub domains: %w", err)
		}
	}

	stubDomains["maesh"] = []string{coreDNSServiceIP}

	configMapData, err := json.Marshal(stubDomains)
	if err != nil {
		return fmt.Errorf("unable to marshal stub domains: %w", err)
	}

	configMap.Data["stubDomains"] = string(configMapData)

	if _, err := c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap); err != nil {
		return err
	}

	return nil
}

// getOrCreateKubeDNSConfigMap parses the deployment and returns the kube-dns ConfigMap. If the associated ConfigMap
// volume is marked as optional and the ConfigMap is not found, this method will create and return the created ConfigMap.
func (c *Client) getOrCreateKubeDNSConfigMap(deployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		if volume.ConfigMap.Name != "kube-dns" {
			continue
		}

		configMap, err := c.getConfigMap(deployment.Namespace, volume.ConfigMap.Name)
		if kerrors.IsNotFound(err) && volume.ConfigMap.Optional != nil && *volume.ConfigMap.Optional {
			configMap = &corev1.ConfigMap{
				Data: make(map[string]string),
				ObjectMeta: metav1.ObjectMeta{
					Name:      volume.ConfigMap.Name,
					Namespace: deployment.Namespace,
				},
			}

			return c.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Create(configMap)
		}

		return configMap, err
	}

	return nil, errors.New("kube-dns configmap not found")
}

// restartPods restarts the pods in a given deployment.
func (c *Client) restartPods(deployment *appsv1.Deployment) error {
	c.logger.Infof("Restarting %q pods", deployment.Name)

	annotations := deployment.Spec.Template.Annotations
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	deployment.Spec.Template.Annotations = annotations

	_, err := c.kubeClient.AppsV1().Deployments(deployment.Namespace).Update(deployment)

	return err
}

// RestoreCoreDNS restores the CoreDNS configuration to pre-install state.
func (c *Client) RestoreCoreDNS() error {
	coreDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	unpatchedConfigMap, err := c.unpatchCoreDNSConfig(coreDNSDeployment)
	if err != nil {
		return fmt.Errorf("unable to unpatch coredns config: %w", err)
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(unpatchedConfigMap.Namespace).Update(unpatchedConfigMap); err != nil {
		return err
	}

	if err := c.restartPods(coreDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) unpatchCoreDNSConfig(deployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	customConfigMap, err := c.getConfigMap(deployment.Namespace, "coredns-custom")

	// For AKS the CoreDNS config have to be removed from the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		delete(customConfigMap.Data, "maesh.server")
		return customConfigMap, nil
	}

	if !kerrors.IsNotFound(err) {
		return nil, fmt.Errorf("unable to get coredns-custom configmap: %w", err)
	}

	coreDNSConfigMap, err := c.getCorefileConfigMap(deployment)
	if err != nil {
		return nil, err
	}

	data := coreDNSConfigMap.Data["Corefile"]

	// Split the data on the header, and save the pre-header data.
	splitData := strings.SplitN(data, coreFileHeader+"\n", 2)
	preData := splitData[0]

	// Split the data on the trailer, and save the post-header data.
	postData := ""
	splitData = strings.SplitN(data, coreFileTrailer+"\n", 2)

	if len(splitData) > 1 {
		postData = splitData[1]
	}

	coreDNSConfigMap.Data["Corefile"] = preData + postData

	return coreDNSConfigMap, nil
}

// RestoreKubeDNS restores the KubeDNS configuration to pre-install state.
func (c *Client) RestoreKubeDNS() error {
	kubeDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get the currently loaded KubeDNS ConfigMap.
	configMap, err := c.getConfigMap(kubeDNSDeployment.Namespace, "kube-dns")
	if err != nil {
		return err
	}

	// Check if stubDomains are still defined.
	stubDomainsStr := configMap.Data["stubDomains"]
	if stubDomainsStr == "" {
		return nil
	}

	stubDomains := make(map[string][]string)
	if err = json.Unmarshal([]byte(stubDomainsStr), &stubDomains); err != nil {
		return fmt.Errorf("unable to unmarshal stubdomains: %w", err)
	}

	// Delete our stubDomain.
	delete(stubDomains, "maesh")

	configMapData, err := json.Marshal(stubDomains)
	if err != nil {
		return err
	}

	configMap.Data["stubDomains"] = string(configMapData)

	if _, err := c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap); err != nil {
		return err
	}

	if err := c.restartPods(kubeDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) getCorefileConfigMap(deployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		configMap, err := c.getConfigMap(deployment.Namespace, volume.ConfigMap.Name)
		if err != nil {
			return nil, err
		}

		if _, exists := configMap.Data["Corefile"]; !exists {
			continue
		}

		return configMap, nil
	}

	return nil, errors.New("corefile configmap not found")
}

func (c *Client) getConfigMap(namespace, name string) (*corev1.ConfigMap, error) {
	configMap, err := c.kubeClient.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	return configMap, nil
}
