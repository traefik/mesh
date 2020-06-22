package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	client k8s.Client
	log    logrus.FieldLogger
}

// NewClient returns an initialized DNSClient object.
func NewClient(log logrus.FieldLogger, client k8s.Client) *Client {
	return &Client{
		client: client,
		log:    log,
	}
}

// CheckDNSProvider checks that the DNS provider that is deployed in the cluster
// is supported and returns it.
func (c *Client) CheckDNSProvider() (Provider, error) {
	c.log.Info("Checking DNS provider")

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
	c.log.Info("Checking CoreDNS")

	deployment, err := c.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.log.Debugf("CoreDNS deployment does not exist in namespace %q", metav1.NamespaceSystem)
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

	c.log.Info("CoreDNS match")

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
	c.log.Info("Checking KubeDNS")

	_, err := c.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.log.Debugf("KubeDNS deployment does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get KubeDNS deployment in namesapce %q: %w", metav1.NamespaceSystem, err)
	}

	c.log.Info("KubeDNS match")

	return true, nil
}

// ConfigureCoreDNS patches the CoreDNS configuration for Maesh.
func (c *Client) ConfigureCoreDNS(coreDNSNamespace, clusterDomain, maeshNamespace string) error {
	c.log.Debug("Patching CoreDNS")

	deployment, err := c.client.KubernetesClient().AppsV1().Deployments(coreDNSNamespace).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	coreConfigMap, err := c.getCorefileConfigMap(deployment)
	if err != nil {
		return err
	}

	c.log.Debug("Patching CoreDNS configmap")

	if err := c.patchCoreDNSConfigMap(coreConfigMap, clusterDomain, maeshNamespace, deployment.Namespace); err != nil {
		return err
	}

	if err := c.restartPods(deployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchCoreDNSConfigMap(coreConfigMap *corev1.ConfigMap, clusterDomain, maeshNamespace, coreNamespace string) error {
	serverBlock := fmt.Sprintf(
		`
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
`,
		clusterDomain,
		strings.Replace(clusterDomain, ".", "\\.", -1),
		maeshNamespace,
		coreFileHeader,
		coreFileTrailer,
	)

	originalBlock := coreConfigMap.Data["Corefile"]

	if strings.Contains(originalBlock, coreFileHeader) {
		// Corefile already contains the maesh block.
		return nil
	}

	newBlock := originalBlock + serverBlock
	coreConfigMap.Data["Corefile"] = newBlock

	if _, err := c.client.KubernetesClient().CoreV1().ConfigMaps(coreNamespace).Update(coreConfigMap); err != nil {
		return err
	}

	return nil
}

// getCorefileConfigMap returns the name of a coreDNS config map.
func (c *Client) getCorefileConfigMap(coreDeployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range coreDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		cfgMap, err := c.client.KubernetesClient().CoreV1().ConfigMaps(coreDeployment.Namespace).Get(volume.ConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		if cfgMap.Data == nil {
			continue
		}

		if _, exists := cfgMap.Data["Corefile"]; !exists {
			continue
		}

		return cfgMap, nil
	}

	return nil, errors.New("corefile configmap not found")
}

// ConfigureKubeDNS patches the KubeDNS configuration for Maesh.
func (c *Client) ConfigureKubeDNS(clusterDomain, maeshNamespace string) error {
	c.log.Debug("Patching KubeDNS")

	deployment, err := c.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	var (
		serviceIP string
		ebo       = backoff.NewConstantBackOff(10 * time.Second)
	)

	c.log.Debug("Getting CoreDNS service IP")

	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		svc, errSvc := c.client.KubernetesClient().CoreV1().Services(maeshNamespace).Get("coredns", metav1.GetOptions{})
		if errSvc != nil {
			return fmt.Errorf("unable get the service %q in namespace %q: %w", "coredns", "maesh", errSvc)
		}
		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("service %q has no clusterIP", "coredns")
		}

		serviceIP = svc.Spec.ClusterIP
		return nil
	}), backoff.WithMaxRetries(ebo, 12)); err != nil {
		return fmt.Errorf("unable get the service %q in namespace %q: %w", "coredns", "maesh", err)
	}

	configMap, err := c.getKubeDNSConfigMap(deployment)
	if err != nil {
		return err
	}

	c.log.Debug("Patching KubeDNS configmap with IP", serviceIP)

	if err := c.patchKubeDNSConfigMap(configMap, deployment.Namespace, serviceIP); err != nil {
		return err
	}

	c.log.Debug("Patching CoreDNS configmap")

	if err := c.ConfigureCoreDNS(maeshNamespace, clusterDomain, maeshNamespace); err != nil {
		return err
	}

	if err := c.restartPods(deployment); err != nil {
		return err
	}

	return nil
}

// getKubeDNSConfigMap parses the deployment and returns the associated configuration configmap.
func (c *Client) getKubeDNSConfigMap(kubeDeployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range kubeDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		cfgMap, err := c.client.KubernetesClient().CoreV1().ConfigMaps(kubeDeployment.Namespace).Get(volume.ConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		return cfgMap, nil
	}

	return nil, errors.New("corefile configmap not found")
}

func (c *Client) patchKubeDNSConfigMap(kubeConfigMap *corev1.ConfigMap, namespace, coreDNSIp string) error {
	originalBlock, exist := kubeConfigMap.Data["stubDomains"]
	if !exist {
		originalBlock = "{}"
	}

	stubDomains := make(map[string][]string)
	if err := json.Unmarshal([]byte(originalBlock), &stubDomains); err != nil {
		return err
	}

	stubDomains["maesh"] = []string{coreDNSIp}

	var newData []byte

	newData, err := json.Marshal(stubDomains)
	if err != nil {
		return err
	}

	if kubeConfigMap.Data == nil {
		kubeConfigMap.Data = make(map[string]string)
	}

	kubeConfigMap.Data["stubDomains"] = string(newData)

	if _, err := c.client.KubernetesClient().CoreV1().ConfigMaps(namespace).Update(kubeConfigMap); err != nil {
		return err
	}

	return nil
}

// restartPods restarts the pods in a given deployment.
func (c *Client) restartPods(deployment *appsv1.Deployment) error {
	c.log.Infof("Restarting %q pods", deployment.Name)

	// Never edit original object, always work with a clone for updates.
	newDeployment := deployment.DeepCopy()
	annotations := newDeployment.Spec.Template.Annotations

	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	newDeployment.Spec.Template.Annotations = annotations
	_, err := c.client.KubernetesClient().AppsV1().Deployments(newDeployment.Namespace).Update(newDeployment)

	return err
}

// RestoreCoreDNS restores the CoreDNS configuration to pre-install state.
func (c *Client) RestoreCoreDNS() error {
	deployment, err := c.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get the currently loaded CoreDNS ConfigMap.
	configMap, err := c.getCorefileConfigMap(deployment)
	if err != nil {
		return err
	}

	data := configMap.Data["Corefile"]

	// Split the data on the header, and save the pre-header data
	splitData := strings.SplitN(data, coreFileHeader+"\n", 2)
	preData := splitData[0]

	// Split the data on the trailer, and save the pre-header data
	postData := ""
	splitData = strings.SplitN(data, coreFileTrailer+"\n", 2)

	if len(splitData) > 1 {
		postData = splitData[1]
	}

	configMap.Data["Corefile"] = preData + postData

	// Update the CoreDNS configmap to the backup.
	if _, err := c.client.KubernetesClient().CoreV1().ConfigMaps(configMap.Namespace).Update(configMap); err != nil {
		return err
	}

	if err := c.restartPods(deployment); err != nil {
		return err
	}

	return nil
}

// RestoreKubeDNS restores the KubeDNS configuration to pre-install state.
func (c *Client) RestoreKubeDNS() error {
	deployment, err := c.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get the currently loaded KubeDNS ConfigMap.
	configMap, err := c.getKubeDNSConfigMap(deployment)
	if err != nil {
		return err
	}

	// Check if stubDomains are still defined.
	originalBlock, exist := configMap.Data["stubDomains"]
	if !exist {
		return nil
	}

	stubDomains := make(map[string][]string)
	if err = json.Unmarshal([]byte(originalBlock), &stubDomains); err != nil {
		return fmt.Errorf("could not unmarshal stubdomains: %w", err)
	}

	// Delete our stubDomain.
	delete(stubDomains, "maesh")

	newData, err := json.Marshal(stubDomains)
	if err != nil {
		return err
	}

	// If there are no stubDomains left, delete the field.
	if string(newData) == "{}" {
		delete(configMap.Data, "stubDomains")
	} else {
		configMap.Data["stubDomains"] = string(newData)
	}

	// Update the KubeDNS configmap to the backup.
	if _, err := c.client.KubernetesClient().CoreV1().ConfigMaps(configMap.Namespace).Update(configMap); err != nil {
		return err
	}

	if err := c.restartPods(deployment); err != nil {
		return err
	}

	return nil
}
