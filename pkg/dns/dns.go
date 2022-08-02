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
	"github.com/traefik/mesh/pkg/safe"
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

	// Maesh config is deprecated and will be removed in the next major release.
	maeshDomain       = "maesh"
	maeshBlockHeader  = "#### Begin Maesh Block"
	maeshBlockTrailer = "#### End Maesh Block"

	traefikMeshDomain       = "traefik.mesh"
	traefikMeshBlockHeader  = "#### Begin Traefik Mesh Block"
	traefikMeshBlockTrailer = "#### End Traefik Mesh Block"
)

var (
	// First CoreDNS version to remove the deprecated upstream and resyncperiod options in the kubernetes plugin.
	versionCoreDNS17 = goversion.Must(goversion.NewVersion("1.7"))

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

	return UnknownDNS, errors.New("no supported DNS service available for installing traefik mesh")
}

func (c *Client) coreDNSMatch(ctx context.Context) (bool, error) {
	c.logger.Debugf("Checking if CoreDNS is installed in namespace %q...", metav1.NamespaceSystem)

	// Most Kubernetes distributions deploy CoreDNS with the following label, so look for it first.
	opts := metav1.ListOptions{
		LabelSelector: "kubernetes.io/name=CoreDNS",
	}
	deployments, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).List(ctx, opts)
	if err != nil {
		return false, fmt.Errorf("unable to list CoreDNS deployments in namespace %q: %w", metav1.NamespaceSystem, err)
	}

	var deployment *appsv1.Deployment
	if len(deployments.Items) == 1 {
		deployment = &deployments.Items[0]
	} else {
		// If we did not find CoreDNS using the annotation (e.g.: with kubeadm), fall back to matching the name of the deployment.
		deployment, err = c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			c.logger.Debug("CoreDNS deployment not found")
			return false, nil
		}

		if err != nil {
			return false, fmt.Errorf("unable to get CoreDNS deployment in namespace %q: %w", metav1.NamespaceSystem, err)
		}
	}

	version, err := c.getCoreDNSVersion(deployment)
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
func (c *Client) ConfigureCoreDNS(ctx context.Context, coreDNSNamespace, clusterDomain, traefikMeshNamespace string) error {
	c.logger.Debugf("Patching ConfigMap %q in namespace %q...", "coredns", coreDNSNamespace)

	coreDNSDeployment, err := c.kubeClient.AppsV1().Deployments(coreDNSNamespace).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	patchedConfigMap, changed, err := c.patchCoreDNSConfig(ctx, coreDNSDeployment, clusterDomain, traefikMeshNamespace)
	if err != nil {
		return fmt.Errorf("unable to patch coredns config: %w", err)
	}

	if !changed {
		c.logger.Infof("CoreDNS ConfigMap %q in namespace %q has already been patched", patchedConfigMap.Name, patchedConfigMap.Namespace)

		return nil
	}

	if _, err = c.kubeClient.CoreV1().ConfigMaps(patchedConfigMap.Namespace).Update(ctx, patchedConfigMap, metav1.UpdateOptions{}); err != nil {
		return err
	}

	c.logger.Infof("CoreDNS ConfigMap %q in namespace %q has successfully been patched", patchedConfigMap.Name, patchedConfigMap.Namespace)

	if err := c.restartPods(ctx, coreDNSDeployment); err != nil {
		return err
	}

	return nil
}

func (c *Client) patchCoreDNSConfig(ctx context.Context, deployment *appsv1.Deployment, clusterDomain, traefikMeshNamespace string) (*corev1.ConfigMap, bool, error) {
	coreDNSVersion, err := c.getCoreDNSVersion(deployment)
	if err != nil {
		return nil, false, err
	}

	customConfigMap, err := c.getConfigMap(ctx, deployment, "coredns-custom")

	// For AKS the CoreDNS config have to be added to the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		// deprecated, will be removed in the next major release.
		corefile, mChanged := addStubDomain(
			customConfigMap.Data["maesh.server"],
			maeshBlockHeader,
			maeshBlockTrailer,
			maeshDomain,
			clusterDomain,
			traefikMeshNamespace,
			coreDNSVersion,
		)
		customConfigMap.Data["maesh.server"] = corefile

		corefile, tChanged := addStubDomain(
			customConfigMap.Data["traefik.mesh.server"],
			traefikMeshBlockHeader,
			traefikMeshBlockTrailer,
			traefikMeshDomain,
			clusterDomain,
			traefikMeshNamespace,
			coreDNSVersion,
		)
		customConfigMap.Data["traefik.mesh.server"] = corefile

		return customConfigMap, mChanged || tChanged, nil
	}

	coreDNSConfigMap, err := c.getConfigMap(ctx, deployment, "coredns")
	if err != nil {
		return nil, false, err
	}

	corefile, mChanged := addStubDomain(
		coreDNSConfigMap.Data["Corefile"],
		maeshBlockHeader,
		maeshBlockTrailer,
		maeshDomain,
		clusterDomain,
		traefikMeshNamespace,
		coreDNSVersion,
	)

	corefile, tChanged := addStubDomain(
		corefile,
		traefikMeshBlockHeader,
		traefikMeshBlockTrailer,
		traefikMeshDomain,
		clusterDomain,
		traefikMeshNamespace,
		coreDNSVersion,
	)

	coreDNSConfigMap.Data["Corefile"] = corefile

	return coreDNSConfigMap, mChanged || tChanged, nil
}

func (c *Client) getCoreDNSVersion(deployment *appsv1.Deployment) (*goversion.Version, error) {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "coredns" {
			continue
		}

		parts := strings.Split(container.Image, ":")

		return goversion.NewVersion(parts[len(parts)-1])
	}

	return nil, fmt.Errorf("unable to get CoreDNS container in deployment %q in namespace %q", deployment.Name, deployment.Namespace)
}

// ConfigureKubeDNS patches the KubeDNS configuration for Traefik Mesh.
func (c *Client) ConfigureKubeDNS(ctx context.Context, clusterDomain, traefikMeshNamespace string) error {
	c.logger.Debugf("Patching ConfigMap %q in namespace %q...", "kube-dns", traefikMeshNamespace)

	kubeDNSDeployment, err := c.kubeClient.AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	var coreDNSServiceIP string

	c.logger.Debugf("Getting ClusterIP for Service %q in namespace %q", "coredns", traefikMeshNamespace)

	operation := func() error {
		svc, svcErr := c.kubeClient.CoreV1().Services(traefikMeshNamespace).Get(ctx, "coredns", metav1.GetOptions{})
		if svcErr != nil {
			return fmt.Errorf("unable to get CoreDNS service in namespace %q: %w", traefikMeshNamespace, err)
		}

		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("coredns service in namespace %q has no ClusterIP", traefikMeshNamespace)
		}

		coreDNSServiceIP = svc.Spec.ClusterIP

		return nil
	}

	if err = backoff.Retry(safe.OperationWithRecover(operation), backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Second), 12)); err != nil {
		return err
	}

	c.logger.Debugf("ClusterIP for Service %q in namespace %q is %q", "coredns", traefikMeshNamespace, coreDNSServiceIP)

	if err := c.patchKubeDNSConfig(ctx, kubeDNSDeployment, coreDNSServiceIP); err != nil {
		return err
	}

	if err := c.ConfigureCoreDNS(ctx, traefikMeshNamespace, clusterDomain, traefikMeshNamespace); err != nil {
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

	// Add our stubDomains.
	// maesh stubDomain is deprecated and will be removed in the next major release.
	stubDomains["maesh"] = []string{coreDNSServiceIP}
	stubDomains["traefik.mesh"] = []string{coreDNSServiceIP}

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

	annotations["traefik-mesh-hash"] = uuid.New().String()
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
	coreDNSConfigMap, err := c.getConfigMap(ctx, deployment, "coredns-custom")

	// For AKS the CoreDNS config have to be removed from the coredns-custom ConfigMap.
	// See https://docs.microsoft.com/en-us/azure/aks/coredns-custom
	if err == nil {
		delete(coreDNSConfigMap.Data, "maesh.server")
		delete(coreDNSConfigMap.Data, "traefik.mesh.server")

		return coreDNSConfigMap, nil
	}

	coreDNSConfigMap, err = c.getConfigMap(ctx, deployment, "coredns")
	if err != nil {
		return nil, err
	}

	corefile := removeStubDomain(
		coreDNSConfigMap.Data["Corefile"],
		maeshBlockHeader,
		maeshBlockTrailer,
	)

	corefile = removeStubDomain(
		corefile,
		traefikMeshBlockHeader,
		traefikMeshBlockTrailer,
	)

	coreDNSConfigMap.Data["Corefile"] = corefile

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

	// Delete our stubDomains.
	// maesh stubDomain is deprecated and will be removed in the next major release.
	delete(stubDomains, "maesh")
	delete(stubDomains, "traefik.mesh")

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

func getStubDomain(config, blockHeader, blockTrailer string) string {
	start := strings.Index(config, blockHeader)
	end := strings.Index(config, blockTrailer)

	if start == -1 || end == -1 {
		return ""
	}

	return config[start : end+len(blockTrailer)]
}

func addStubDomain(config, blockHeader, blockTrailer, domain, clusterDomain, traefikMeshNamespace string, coreDNSVersion *goversion.Version) (string, bool) {
	existingStubDomain := getStubDomain(config, blockHeader, blockTrailer)

	if existingStubDomain != "" {
		config = removeStubDomain(config, blockHeader, blockTrailer)
	}

	stubDomainFormat := `%[4]s
%[7]s:53 {
    errors
    rewrite continue {
        name regex ([a-zA-Z0-9-_]*)\.([a-zv0-9-_]*)\.%[7]s %[3]s-{1}-6d61657368-{2}.%[3]s.svc.%[1]s
        answer name %[3]s-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\.%[3]s\.svc\.%[2]s {1}.{2}.%[7]s
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
%[5]s`

	upstream := ""

	if coreDNSVersion.Core().LessThan(versionCoreDNS17) {
		upstream = "upstream"
	}

	stubDomain := fmt.Sprintf(stubDomainFormat,
		clusterDomain,
		strings.ReplaceAll(clusterDomain, ".", "\\."),
		traefikMeshNamespace,
		blockHeader,
		blockTrailer,
		upstream,
		domain,
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
