package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/maesh/pkg/safe"
	"github.com/google/uuid"
	goversion "github.com/hashicorp/go-version"
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

var versionCoreDNS17 = goversion.Must(goversion.NewVersion("1.7"))

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

// CheckDNSProvider checks that the DNS provider deployed in the cluster is supported and returns it.
func (c *Client) CheckDNSProvider(ctx context.Context) (Provider, error) {
	c.logger.Info("Checking DNS provider")

	match, err := c.coreDNSMatch(ctx)
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return CoreDNS, nil
	}

	match, err = c.kubeDNSMatch(ctx)
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return KubeDNS, nil
	}

	return UnknownDNS, errors.New("no supported DNS service available for installing maesh")
}

func (c *Client) coreDNSMatch(ctx context.Context) (bool, error) {
	c.logger.Info("Checking CoreDNS")

	deployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.logger.Debugf("CoreDNS deployment does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get CoreDNS deployment in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	version, err := c.getCoreDNSVersion(deployment)
	if err != nil {
		return false, err
	}

	versionConstraint, err := goversion.NewConstraint(">= 1.3, < 1.8")
	if err != nil {
		return false, err
	}

	if !versionConstraint.Check(version) {
		return false, fmt.Errorf("unsupported CoreDNS version %q", version)
	}

	c.logger.Info("CoreDNS match")

	return true, nil
}

func (c *Client) kubeDNSMatch(ctx context.Context) (bool, error) {
	c.logger.Info("Checking KubeDNS")

	_, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
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
func (c *Client) ConfigureCoreDNS(ctx context.Context, coreDNSNamespace, clusterDomain, maeshNamespace string) error {
	c.logger.Debug("Patching CoreDNS")

	coreDNSDeployment, err := c.kubeClient.AppsV1().Deployments(coreDNSNamespace).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	c.logger.Debug("Patching CoreDNS ConfigMap")

	patchedConfigMap, err := c.patchCoreDNSConfig(ctx, coreDNSDeployment, clusterDomain, maeshNamespace)
	if err != nil {
		return fmt.Errorf("unable to patch coredns config: %w", err)
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(patchedConfigMap.Namespace).Update(ctx, patchedConfigMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	if err := c.restartPods(ctx, coreDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchCoreDNSConfig(ctx context.Context, deployment *appsv1.Deployment, clusterDomain, maeshNamespace string) (*corev1.ConfigMap, error) {
	coreDNSVersion, err := c.getCoreDNSVersion(deployment)
	if err != nil {
		return nil, err
	}

	customConfigMap, err := c.getConfigMap(ctx, deployment, "coredns-custom")

	// For AKS the CoreDNS config have to be added to the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		customConfigMap.Data["maesh.server"] = c.addMaeshStubDomain(
			clusterDomain,
			maeshNamespace,
			"",
			coreDNSVersion,
		)

		return customConfigMap, nil
	}

	coreDNSConfigMap, err := c.getConfigMap(ctx, deployment, "coredns")
	if err != nil {
		return nil, err
	}

	coreDNSConfigMap.Data["Corefile"] = c.addMaeshStubDomain(
		clusterDomain,
		maeshNamespace,
		coreDNSConfigMap.Data["Corefile"],
		coreDNSVersion,
	)

	return coreDNSConfigMap, nil
}

func (c *Client) addMaeshStubDomain(clusterDomain, maeshNamespace, coreDNSConfig string, coreDNSVersion *goversion.Version) string {
	// config already contains the maesh block.
	if strings.Contains(coreDNSConfig, coreFileHeader) {
		return coreDNSConfig
	}

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
        %[6]s
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
	upstream := ""

	if coreDNSVersion.LessThan(versionCoreDNS17) {
		upstream = "upstream"
	}

	stubDomain := fmt.Sprintf(stubDomainFormat,
		clusterDomain,
		strings.Replace(clusterDomain, ".", "\\.", -1),
		maeshNamespace,
		coreFileHeader,
		coreFileTrailer,
		upstream,
	)

	return coreDNSConfig + stubDomain
}

func (c *Client) getCoreDNSVersion(deployment *appsv1.Deployment) (*goversion.Version, error) {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "coredns" {
			continue
		}

		parts := strings.Split(container.Image, ":")

		return goversion.NewVersion(parts[len(parts)-1])
	}

	return nil, fmt.Errorf("unable to get CoreDNS container in deployment %q/%q", deployment.Namespace, deployment.Name)
}

// ConfigureKubeDNS patches the KubeDNS configuration for Maesh.
func (c *Client) ConfigureKubeDNS(ctx context.Context, clusterDomain, maeshNamespace string) error {
	c.logger.Debug("Patching KubeDNS")

	kubeDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	c.logger.Debug("Getting CoreDNS service IP")

	var coreDNSServiceIP string

	operation := func() error {
		svc, svcErr := c.kubeClient.CoreV1().Services(maeshNamespace).Get(ctx, "coredns", metav1.GetOptions{})
		if svcErr != nil {
			return fmt.Errorf("unable to get coredns service in namespace %q: %w", maeshNamespace, err)
		}

		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("coredns service in namespace %q has no clusterip", maeshNamespace)
		}

		coreDNSServiceIP = svc.Spec.ClusterIP

		return nil
	}

	if err = backoff.Retry(safe.OperationWithRecover(operation), backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Second), 12)); err != nil {
		return err
	}

	c.logger.Debugf("Patching KubeDNS ConfigMap with CoreDNS service IP %q", coreDNSServiceIP)

	if err := c.patchKubeDNSConfig(ctx, kubeDNSDeployment, coreDNSServiceIP); err != nil {
		return err
	}

	if err := c.ConfigureCoreDNS(ctx, maeshNamespace, clusterDomain, maeshNamespace); err != nil {
		return err
	}

	if err := c.restartPods(ctx, kubeDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchKubeDNSConfig(ctx context.Context, deployment *appsv1.Deployment, coreDNSServiceIP string) error {
	configMap, err := c.getOrCreateConfigMap(ctx, deployment, "kube-dns")
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

	if _, err := c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

// restartPods restarts the pods in a given deployment.
func (c *Client) restartPods(ctx context.Context, deployment *appsv1.Deployment) error {
	c.logger.Infof("Restarting %q pods", deployment.Name)

	annotations := deployment.Spec.Template.Annotations
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	deployment.Spec.Template.Annotations = annotations

	_, err := c.kubeClient.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})

	return err
}

// RestoreCoreDNS restores the CoreDNS configuration to pre-install state.
func (c *Client) RestoreCoreDNS(ctx context.Context) error {
	coreDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	unpatchedConfigMap, err := c.unpatchCoreDNSConfig(ctx, coreDNSDeployment)
	if err != nil {
		return fmt.Errorf("unable to unpatch coredns config: %w", err)
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(unpatchedConfigMap.Namespace).Update(ctx, unpatchedConfigMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	if err := c.restartPods(ctx, coreDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) unpatchCoreDNSConfig(ctx context.Context, deployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	customConfigMap, err := c.getConfigMap(ctx, deployment, "coredns-custom")

	// For AKS the CoreDNS config have to be removed from the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		delete(customConfigMap.Data, "maesh.server")
		return customConfigMap, nil
	}

	coreDNSConfigMap, err := c.getConfigMap(ctx, deployment, "coredns")
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
func (c *Client) RestoreKubeDNS(ctx context.Context) error {
	kubeDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get the currently loaded KubeDNS ConfigMap.
	configMap, err := c.getConfigMap(ctx, kubeDNSDeployment, "kube-dns")
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

	if _, err := c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	if err := c.restartPods(ctx, kubeDNSDeployment); err != nil {
		return err
	}

	return nil
}

// getOrCreateConfigMap parses the deployment and returns the ConfigMap with the given name. This method will create the
// corresponding ConfigMap if the associated volume is marked as optional and the ConfigMap is not found.
func (c *Client) getOrCreateConfigMap(ctx context.Context, deployment *appsv1.Deployment, name string) (*corev1.ConfigMap, error) {
	volumeSrc, err := getConfigMapVolumeSource(deployment, name)
	if err != nil {
		return nil, err
	}

	configMap, err := c.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Get(ctx, volumeSrc.Name, metav1.GetOptions{})

	if kerrors.IsNotFound(err) && volumeSrc.Optional != nil && *volumeSrc.Optional {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: deployment.Namespace,
			},
		}

		configMap, err = c.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
	}

	if err != nil {
		return nil, err
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	return configMap, err
}

// getConfigMap parses the deployment and returns the ConfigMap with the given name.
func (c *Client) getConfigMap(ctx context.Context, deployment *appsv1.Deployment, name string) (*corev1.ConfigMap, error) {
	volumeSrc, err := getConfigMapVolumeSource(deployment, name)
	if err != nil {
		return nil, err
	}

	configMap, err := c.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Get(ctx, volumeSrc.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	return configMap, nil
}

// getConfigMapVolumeSource returns the ConfigMapVolumeSource corresponding to the ConfigMap with the given name.
func getConfigMapVolumeSource(deployment *appsv1.Deployment, name string) (*corev1.ConfigMapVolumeSource, error) {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		if volume.ConfigMap.Name != name {
			continue
		}

		return volume.ConfigMap, nil
	}

	return nil, fmt.Errorf("configmap %q cannot be found", name)
}
