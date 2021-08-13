package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	goversion "github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/pkg/safe"
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

	blockHeader  = "#### Begin Traefik Mesh Block"
	blockTrailer = "#### End Traefik Mesh Block"
)

var (
	versionCoreDNS14 = goversion.Must(goversion.NewVersion("1.4"))

	// Currently supported CoreDNS versions range.
	versionCoreDNSMin = goversion.Must(goversion.NewVersion("1.3"))
	versionCoreDNSMax = goversion.Must(goversion.NewVersion("1.9"))
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

// CheckDNSProvider checks that the DNS provider deployed in the cluster is supported and returns it.
func (c *Client) CheckDNSProvider(ctx context.Context) (Provider, error) {
	c.logger.Debug("Detecting DNS provider...")

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

	return UnknownDNS, errors.New("no supported DNS service available")
}

func (c *Client) coreDNSMatch(ctx context.Context) (bool, error) {
	c.logger.Debugf("Checking if CoreDNS is installed in namespace %q...", metav1.NamespaceSystem)

	dnsDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.logger.Debug("CoreDNS deployment not found")
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get CoreDNS deployment in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	version, err := getCoreDNSVersion(dnsDeployment)
	if err != nil {
		return false, err
	}

	if !(version.Core().GreaterThanOrEqual(versionCoreDNSMin) && version.Core().LessThan(versionCoreDNSMax)) {
		c.logger.Debugf(`CoreDNS version is not supported, must satisfy ">= %s, < %s", got %q`, versionCoreDNSMin, versionCoreDNSMax, version)

		return false, fmt.Errorf("unsupported CoreDNS version %q", version)
	}

	c.logger.Debugf("CoreDNS %q has been detected", version)

	return true, nil
}

func (c *Client) kubeDNSMatch(ctx context.Context) (bool, error) {
	c.logger.Debugf("Checking if KubeDNS is installed in namespace %q...", metav1.NamespaceSystem)

	_, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		c.logger.Debug("KubeDNS deployment not found")
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get KubeDNS deployment in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	c.logger.Debug("KubeDNS has been detected")

	return true, nil
}

// ConfigureCoreDNS patches the CoreDNS configuration for Traefik Mesh.
func (c *Client) ConfigureCoreDNS(ctx context.Context, dnsServiceNamespace, dnsServiceName string, dnsServicePort int32) error {
	dnsDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	dnsServiceIP, err := c.getServiceIP(ctx, dnsServiceNamespace, dnsServiceName)
	if err != nil {
		return fmt.Errorf("unable to get ClusterIP of DNS service %q in namespace %q: %w", dnsServiceName, dnsServiceNamespace, err)
	}

	configMap, changed, err := c.patchCoreDNSConfig(ctx, dnsDeployment, dnsServiceIP, dnsServicePort)
	if err != nil {
		return fmt.Errorf("unable to patch coredns config: %w", err)
	}

	if !changed {
		c.logger.Infof("CoreDNS ConfigMap %q in namespace %q has already been patched", configMap.Name, configMap.Namespace)

		return nil
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	c.logger.Infof("CoreDNS ConfigMap %q in namespace %q has successfully been patched", configMap.Name, configMap.Namespace)

	if err := c.restartPods(ctx, dnsDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchCoreDNSConfig(ctx context.Context, deployment *appsv1.Deployment, dnsServiceIP string, dnsServicePort int32) (*corev1.ConfigMap, bool, error) {
	version, err := getCoreDNSVersion(deployment)
	if err != nil {
		return nil, false, err
	}

	customConfigMap, err := c.getConfigMap(ctx, deployment, "coredns-custom")

	// For AKS the CoreDNS config have to be added to the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		corefile, changed := addStubDomain(
			customConfigMap.Data["traefik.mesh.server"],
			blockHeader,
			blockTrailer,
			dnsServiceIP,
			dnsServicePort,
			version,
		)

		customConfigMap.Data["traefik.mesh.server"] = corefile

		return customConfigMap, changed, nil
	}

	coreDNSConfigMap, err := c.getConfigMap(ctx, deployment, "coredns")
	if err != nil {
		return nil, false, err
	}

	corefile, changed := addStubDomain(
		coreDNSConfigMap.Data["Corefile"],
		blockHeader,
		blockTrailer,
		dnsServiceIP,
		dnsServicePort,
		version,
	)

	coreDNSConfigMap.Data["Corefile"] = corefile

	return coreDNSConfigMap, changed, nil
}

// ConfigureKubeDNS patches the KubeDNS configuration for Traefik Mesh.
func (c *Client) ConfigureKubeDNS(ctx context.Context, dnsServiceNamespace, dnsServiceName string, dnsServicePort int32) error {
	dnsDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	dnsServiceIP, err := c.getServiceIP(ctx, dnsServiceNamespace, dnsServiceName)
	if err != nil {
		return fmt.Errorf("unable to get ClusterIP of DNS service %q in namespace %q: %w", dnsServiceName, dnsServiceNamespace, err)
	}

	c.logger.Debugf("ClusterIP for Service %q in namespace %q is %q", "coredns", metav1.NamespaceSystem, dnsServiceIP)

	if err := c.patchKubeDNSConfig(ctx, dnsDeployment, dnsServiceIP, dnsServicePort); err != nil {
		return err
	}

	if err := c.restartPods(ctx, dnsDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchKubeDNSConfig(ctx context.Context, deployment *appsv1.Deployment, dnsServiceIP string, dnsServicePort int32) error {
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

	// Add our stubDomain.
	stubDomains["traefik.mesh"] = []string{fmt.Sprintf("%s:%d", dnsServiceIP, dnsServicePort)}

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

// RestoreCoreDNS restores the CoreDNS configuration to pre-install state.
func (c *Client) RestoreCoreDNS(ctx context.Context) error {
	dnsDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	configMap, err := c.unpatchCoreDNSConfig(ctx, dnsDeployment)
	if err != nil {
		return fmt.Errorf("unable to unpatch coredns config: %w", err)
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	if err := c.restartPods(ctx, dnsDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) unpatchCoreDNSConfig(ctx context.Context, deployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	coreDNSConfigMap, err := c.getConfigMap(ctx, deployment, "coredns-custom")

	// For AKS the CoreDNS config have to be removed from the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		delete(coreDNSConfigMap.Data, "traefik.mesh.server")

		return coreDNSConfigMap, nil
	}

	coreDNSConfigMap, err = c.getConfigMap(ctx, deployment, "coredns")
	if err != nil {
		return nil, err
	}

	corefile := removeStubDomain(
		coreDNSConfigMap.Data["Corefile"],
		blockHeader,
		blockTrailer,
	)

	coreDNSConfigMap.Data["Corefile"] = corefile

	return coreDNSConfigMap, nil
}

// RestoreKubeDNS restores the KubeDNS configuration to pre-install state.
func (c *Client) RestoreKubeDNS(ctx context.Context) error {
	dnsDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get the currently loaded KubeDNS ConfigMap.
	configMap, err := c.getConfigMap(ctx, dnsDeployment, "kube-dns")
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
	delete(stubDomains, "traefik.mesh")

	configMapData, err := json.Marshal(stubDomains)
	if err != nil {
		return err
	}

	configMap.Data["stubDomains"] = string(configMapData)

	if _, err := c.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	if err := c.restartPods(ctx, dnsDeployment); err != nil {
		return err
	}

	return nil
}

// getOrCreateConfigMap parses the deployment and returns the ConfigMap with the given name. This method will create the
// corresponding ConfigMap if the associated volume is marked as optional and the ConfigMap is not found.
func (c *Client) getOrCreateConfigMap(ctx context.Context, deployment *appsv1.Deployment, name string) (*corev1.ConfigMap, error) {
	volume, err := getConfigMapVolume(deployment, name)
	if err != nil {
		return nil, err
	}

	configMap, err := c.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Get(ctx, volume.Name, metav1.GetOptions{})

	if kerrors.IsNotFound(err) && volume.Optional != nil && *volume.Optional {
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
	volume, err := getConfigMapVolume(deployment, name)
	if err != nil {
		return nil, err
	}

	configMap, err := c.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Get(ctx, volume.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	return configMap, nil
}

// restartPods restarts the pods in a given deployment.
func (c *Client) restartPods(ctx context.Context, deployment *appsv1.Deployment) error {
	c.logger.Infof("Restarting %q pods", deployment.Name)

	annotations := deployment.Spec.Template.Annotations
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["traefik-mesh-hash"] = uuid.New().String()
	deployment.Spec.Template.Annotations = annotations

	_, err := c.kubeClient.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})

	return err
}

// getServiceIP returns the ClusterIP of the given service name in the given namespace.
func (c *Client) getServiceIP(ctx context.Context, namespace, name string) (string, error) {
	var clusterIP string

	operation := func() error {
		service, err := c.kubeClient.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("unable to get service %q in namespace %q: %w", name, namespace, err)
		}

		if service.Spec.ClusterIP == "" {
			return fmt.Errorf("service %q in namespace %q has no ClusterIP", name, namespace)
		}

		clusterIP = service.Spec.ClusterIP

		return nil
	}

	if err := backoff.Retry(safe.OperationWithRecover(operation), backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Second), 12)); err != nil {
		return "", err
	}

	return clusterIP, nil
}

// getConfigMapVolume returns the ConfigMapVolumeSource corresponding to the ConfigMap with the given name.
func getConfigMapVolume(deployment *appsv1.Deployment, name string) (*corev1.ConfigMapVolumeSource, error) {
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

func getStubDomain(config, blockHeader, blockTrailer string) string {
	start := strings.Index(config, blockHeader)
	end := strings.Index(config, blockTrailer)

	if start == -1 || end == -1 {
		return ""
	}

	return config[start : end+len(blockTrailer)]
}

func addStubDomain(config, blockHeader, blockTrailer, dnsServiceIP string, dnsServicePort int32, coreDNSVersion *goversion.Version) (string, bool) {
	existingStubDomain := getStubDomain(config, blockHeader, blockTrailer)
	if existingStubDomain != "" {
		config = removeStubDomain(config, blockHeader, blockTrailer)
	}

	stubDomainFormat := `%[4]s
traefik.mesh:53 {
    errors
    cache 30
    %[1]s . %[2]s:%[3]d
}
%[5]s`

	forward := "forward"
	if coreDNSVersion.LessThan(versionCoreDNS14) {
		forward = "proxy"
	}

	stubDomain := fmt.Sprintf(stubDomainFormat,
		forward,
		dnsServiceIP,
		dnsServicePort,
		blockHeader,
		blockTrailer,
	)

	return config + "\n" + stubDomain + "\n", existingStubDomain != stubDomain
}

func removeStubDomain(config, blockHeader, blockTrailer string) string {
	if !strings.Contains(config, blockHeader) {
		return config
	}

	// Split the data on the header, and save the pre-header data.
	splitData := strings.SplitN(config, blockHeader+"\n", 2)
	preData := splitData[0]

	// Split the data on the trailer, and save the post-header data.
	postData := ""

	splitData = strings.SplitN(config, blockTrailer+"\n", 2)
	if len(splitData) > 1 {
		postData = splitData[1]
	}

	return preData + postData
}

func getCoreDNSVersion(deployment *appsv1.Deployment) (*goversion.Version, error) {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "coredns" {
			continue
		}

		parts := strings.Split(container.Image, ":")

		return goversion.NewVersion(parts[len(parts)-1])
	}

	return nil, fmt.Errorf("unable to get CoreDNS container in deployment %q in namespace %q", deployment.Name, deployment.Namespace)
}
