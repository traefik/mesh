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
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSProvider represents a DNS provider.
type DNSProvider int

// Supported DNS providers.
const (
	UnknownDNS DNSProvider = iota
	CoreDNS
	KubeDNS
)

var (
	supportedCoreDNSVersions = []string{
		"1.3",
		"1.4",
		"1.5",
		"1.6",
	}
	coreFileHeader  = "#### Begin Maesh Block"
	coreFileTrailer = "#### End Maesh Block"
)

// DNSClient holds the client for interacting with the k8s DNS system.
type DNSClient struct {
	client k8s.Client
	log    logrus.FieldLogger
}

// NewDNSClient returns an initialized DNSClient object.
func NewDNSClient(log logrus.FieldLogger, client k8s.Client) *DNSClient {
	return &DNSClient{
		client: client,
		log:    log,
	}
}

// CheckDNSProvider checks that the DNS provider that is deployed in the cluster
// is supported and returns it.
func (d *DNSClient) CheckDNSProvider() (DNSProvider, error) {
	d.log.Info("Checking DNS provider")

	match, err := d.coreDNSMatch()
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return CoreDNS, nil
	}

	match, err = d.kubeDNSMatch()
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return KubeDNS, nil
	}

	return UnknownDNS, fmt.Errorf("no core DNS service available for installing maesh: %w", err)
}

func (d *DNSClient) coreDNSMatch() (bool, error) {
	d.log.Info("Checking CoreDNS")

	deployment, err := d.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if kubeerror.IsNotFound(err) {
		d.log.Debugf("CoreDNS does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %w", "coredns", metav1.NamespaceSystem, err)
	}

	var version string

	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name != "coredns" {
			continue
		}

		sp := strings.Split(c.Image, ":")
		version = sp[len(sp)-1]
	}

	if !isCoreDNSVersionSupported(version) {
		return false, fmt.Errorf("unsupported CoreDNS version %q, (supported versions are: %s)", version, strings.Join(supportedCoreDNSVersions, ","))
	}

	d.log.Info("CoreDNS match")

	return true, nil
}

func isCoreDNSVersionSupported(versionLine string) bool {
	for _, v := range supportedCoreDNSVersions {
		if strings.Contains(versionLine, v) || strings.Contains(versionLine, "v"+v) {
			return true
		}
	}

	return false
}

func (d *DNSClient) kubeDNSMatch() (bool, error) {
	d.log.Info("Checking KubeDNS")

	_, err := d.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if kubeerror.IsNotFound(err) {
		d.log.Debugf("KubeDNS does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %w", "kube-dns", metav1.NamespaceSystem, err)
	}

	d.log.Info("KubeDNS match")

	return true, nil
}

// ConfigureCoreDNS patches the CoreDNS configuration for Maesh.
func (d *DNSClient) ConfigureCoreDNS(clusterDomain, maeshNamespace string) error {
	d.log.Debug("Patching CoreDNS")

	deployment, err := d.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	coreConfigMap, err := d.GetCorefileConfigMap(deployment)
	if err != nil {
		return err
	}

	d.log.Debug("Patching CoreDNS configmap")

	if err := d.patchCoreDNSConfigMap(coreConfigMap, clusterDomain, maeshNamespace, deployment.Namespace); err != nil {
		return err
	}

	if err := d.RestartPods(deployment); err != nil {
		return err
	}

	return nil
}

func (d *DNSClient) patchCoreDNSConfigMap(coreConfigMap *corev1.ConfigMap, clusterDomain, maeshNamespace, coreNamespace string) error {
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

	if _, err := d.client.KubernetesClient().CoreV1().ConfigMaps(coreNamespace).Update(coreConfigMap); err != nil {
		return err
	}

	return nil
}

// GetCorefileConfigMap returns the name of a coreDNS config map.
func (d *DNSClient) GetCorefileConfigMap(coreDeployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range coreDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		cfgMap, err := d.client.KubernetesClient().CoreV1().ConfigMaps(coreDeployment.Namespace).Get(volume.ConfigMap.Name, metav1.GetOptions{})
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
func (d *DNSClient) ConfigureKubeDNS(maeshNamespace string) error {
	d.log.Debug("Patching KubeDNS")

	deployment, err := d.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	var (
		serviceIP string
		ebo       = backoff.NewConstantBackOff(10 * time.Second)
	)

	d.log.Debug("Getting CoreDNS service IP")

	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		svc, errSvc := d.client.KubernetesClient().CoreV1().Services(maeshNamespace).Get("coredns", metav1.GetOptions{})
		if errSvc != nil {
			return fmt.Errorf("unable get the service %q in namespace %q: %w", "coredns", "maesh", errSvc)
		}
		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("service %q has no clusterIP", "coredns")
		}

		serviceIP = svc.Spec.ClusterIP
		return nil
	}), ebo); err != nil {
		return fmt.Errorf("unable get the service %q in namespace %q: %w", "coredns", "maesh", err)
	}

	configMap, err := d.GetKubeDNSConfigMap(deployment)
	if err != nil {
		return err
	}

	d.log.Debug("Patching KubeDNS configmap with IP", serviceIP)

	if err := d.patchKubeDNSConfigMap(configMap, deployment.Namespace, serviceIP); err != nil {
		return err
	}

	if err := d.RestartPods(deployment); err != nil {
		return err
	}

	return nil
}

// GetKubeDNSConfigMap parses the deployment and returns the associated configuration configmap.
func (d *DNSClient) GetKubeDNSConfigMap(kubeDeployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range kubeDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		cfgMap, err := d.client.KubernetesClient().CoreV1().ConfigMaps(kubeDeployment.Namespace).Get(volume.ConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		return cfgMap, nil
	}

	return nil, errors.New("corefile configmap not found")
}

func (d *DNSClient) patchKubeDNSConfigMap(kubeConfigMap *corev1.ConfigMap, namespace, coreDNSIp string) error {
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

	if _, err := d.client.KubernetesClient().CoreV1().ConfigMaps(namespace).Update(kubeConfigMap); err != nil {
		return err
	}

	return nil
}

// RestartPods restarts the pods in a given deployment.
func (d *DNSClient) RestartPods(deployment *appsv1.Deployment) error {
	d.log.Infof("Restarting %q pods", deployment.Name)

	// Never edit original object, always work with a clone for updates.
	newDeployment := deployment.DeepCopy()
	annotations := newDeployment.Spec.Template.Annotations

	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	newDeployment.Spec.Template.Annotations = annotations
	_, err := d.client.KubernetesClient().AppsV1().Deployments(newDeployment.Namespace).Update(newDeployment)

	return err
}
