package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/google/uuid"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// DNSProvider represents a DNS provider.
type DNSProvider int

// Supported DNS providers.
const (
	UnknownDNS DNSProvider = iota
	CoreDNS
	KubeDNS
)

var supportedCoreDNSVersions = []string{
	"1.3",
	"1.4",
	"1.5",
	"1.6",
}

// Preparer is an interface for the prepare methods.
type Preparer interface {
	CheckDNSProvider() (DNSProvider, error)
	StartInformers(acl bool) error
	ConfigureCoreDNS(clusterDomain, namespace string) error
	ConfigureKubeDNS() error
}

// Ensure the Prepare fits the Preparer interface.
var _ Preparer = (*Prepare)(nil)

// Prepare holds the clients for the various resource controllers.
type Prepare struct {
	client k8s.Client
	log    logrus.FieldLogger
}

// NewPrepare returns an initialized prepare object.
func NewPrepare(log logrus.FieldLogger, client k8s.Client) Preparer {
	return &Prepare{
		client: client,
		log:    log,
	}
}

// CheckDNSProvider checks that the DNS provider that is deployed in the cluster
// is supported and returns it.
func (p *Prepare) CheckDNSProvider() (DNSProvider, error) {
	p.log.Info("Checking DNS provider")

	match, err := p.coreDNSMatch()
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return CoreDNS, nil
	}

	match, err = p.kubeDNSMatch()
	if err != nil {
		return UnknownDNS, err
	}

	if match {
		return KubeDNS, nil
	}

	return UnknownDNS, fmt.Errorf("no core DNS service available for installing maesh: %w", err)
}

func (p *Prepare) coreDNSMatch() (bool, error) {
	p.log.Info("Checking CoreDNS")

	deployment, err := p.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if kubeerror.IsNotFound(err) {
		p.log.Debugf("CoreDNS does not exist in namespace %q", metav1.NamespaceSystem)
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

	p.log.Info("CoreDNS match")

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

func (p *Prepare) kubeDNSMatch() (bool, error) {
	p.log.Info("Checking KubeDNS")

	_, err := p.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if kubeerror.IsNotFound(err) {
		p.log.Debugf("KubeDNS does not exist in namespace %q", metav1.NamespaceSystem)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %w", "kube-dns", metav1.NamespaceSystem, err)
	}

	p.log.Info("KubeDNS match")

	return true, nil
}

// ConfigureCoreDNS patches the CoreDNS configuration for Maesh.
func (p *Prepare) ConfigureCoreDNS(clusterDomain, maeshNamespace string) error {
	p.log.Debug("Patching CoreDNS")

	deployment, err := p.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	coreConfigMap, err := p.getCorefileConfigMap(deployment)
	if err != nil {
		return err
	}

	if isPatched(coreConfigMap) {
		// CoreDNS has already been patched.
		p.log.Debug("Configmap already patched")
		return nil
	}

	p.log.Debug("Patching CoreDNS configmap")

	if err := p.patchCoreDNSConfigMap(coreConfigMap, clusterDomain, maeshNamespace, deployment.Namespace); err != nil {
		return err
	}

	if err := p.restartPods(deployment); err != nil {
		return err
	}

	return nil
}

func (p *Prepare) patchCoreDNSConfigMap(coreConfigMap *corev1.ConfigMap, clusterDomain, maeshNamespace, coreNamespace string) error {
	serverBlock := fmt.Sprintf(
		`
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
`,
		clusterDomain,
		strings.Replace(clusterDomain, ".", "\\.", -1),
		maeshNamespace,
	)

	originalBlock := coreConfigMap.Data["Corefile"]
	newBlock := originalBlock + serverBlock
	coreConfigMap.Data["Corefile"] = newBlock

	if coreConfigMap.ObjectMeta.Labels == nil {
		coreConfigMap.ObjectMeta.Labels = make(map[string]string)
	}

	coreConfigMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err := p.client.GetKubernetesClient().CoreV1().ConfigMaps(coreNamespace).Update(coreConfigMap); err != nil {
		return err
	}

	return nil
}

// getCorefileConfigMap returns the name of a coreDNS config map.
func (p *Prepare) getCorefileConfigMap(coreDeployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range coreDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		cfgMap, err := p.client.GetKubernetesClient().CoreV1().ConfigMaps(coreDeployment.Namespace).Get(volume.ConfigMap.Name, metav1.GetOptions{})
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
func (p *Prepare) ConfigureKubeDNS() error {
	p.log.Debug("Patching KubeDNS")

	deployment, err := p.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	var (
		serviceIP string
		ebo       = backoff.NewConstantBackOff(10 * time.Second)
	)

	p.log.Debug("Getting CoreDNS service IP")

	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		svc, errSvc := p.client.GetKubernetesClient().CoreV1().Services("maesh").Get("coredns", metav1.GetOptions{})
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

	configMap, err := p.getKubeDNSConfigMap(deployment)
	if err != nil {
		return err
	}

	if isPatched(configMap) {
		p.log.Debug("Configmap already patched")

		return nil
	}

	p.log.Debug("Patching KubeDNS configmap with IP", serviceIP)

	if err := p.patchKubeDNSConfigMap(configMap, deployment.Namespace, serviceIP); err != nil {
		return err
	}

	if err := p.restartPods(deployment); err != nil {
		return err
	}

	return nil
}

func (p *Prepare) getKubeDNSConfigMap(kubeDeployment *appsv1.Deployment) (*corev1.ConfigMap, error) {
	for _, volume := range kubeDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}

		cfgMap, err := p.client.GetKubernetesClient().CoreV1().ConfigMaps(kubeDeployment.Namespace).Get(volume.ConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		return cfgMap, nil
	}

	return nil, errors.New("corefile configmap not found")
}

func (p *Prepare) patchKubeDNSConfigMap(kubeConfigMap *corev1.ConfigMap, namespace, coreDNSIp string) error {
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

	if len(kubeConfigMap.ObjectMeta.Labels) == 0 {
		kubeConfigMap.ObjectMeta.Labels = make(map[string]string)
	}

	kubeConfigMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err := p.client.GetKubernetesClient().CoreV1().ConfigMaps(namespace).Update(kubeConfigMap); err != nil {
		return err
	}

	return nil
}

// StartInformers checks if the required informers can start and sync in a reasonable time.
func (p *Prepare) StartInformers(acl bool) error {
	stopCh := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.startBaseInformers(ctx, stopCh); err != nil {
		return err
	}

	if !acl {
		return nil
	}

	if err := p.startACLInformers(ctx, stopCh); err != nil {
		return err
	}

	return nil
}

func (p *Prepare) startBaseInformers(ctx context.Context, stopCh <-chan struct{}) error {
	// Create a new SharedInformerFactory, and register the event handler to informers.
	kubeFactory := informers.NewSharedInformerFactoryWithOptions(p.client.GetKubernetesClient(), k8s.ResyncPeriod)
	kubeFactory.Core().V1().Services().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Core().V1().Endpoints().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Start(stopCh)

	for t, ok := range kubeFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	splitFactory := splitinformer.NewSharedInformerFactoryWithOptions(p.client.GetSplitClient(), k8s.ResyncPeriod)
	splitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	splitFactory.Start(stopCh)

	for t, ok := range splitFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	return nil
}

func (p *Prepare) startACLInformers(ctx context.Context, stopCh <-chan struct{}) error {
	// Create new SharedInformerFactories, and register the event handler to informers.
	accessFactory := accessinformer.NewSharedInformerFactoryWithOptions(p.client.GetAccessClient(), k8s.ResyncPeriod)
	accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	accessFactory.Start(stopCh)

	for t, ok := range accessFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	specsFactory := specsinformer.NewSharedInformerFactoryWithOptions(p.client.GetSpecsClient(), k8s.ResyncPeriod)
	specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	specsFactory.Start(stopCh)

	for t, ok := range specsFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	// Create a new SharedInformerFactory, and register the event handler to informers.
	kubeFactory := informers.NewSharedInformerFactoryWithOptions(p.client.GetKubernetesClient(), k8s.ResyncPeriod)
	kubeFactory.Core().V1().Pods().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Start(stopCh)

	for t, ok := range kubeFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	return nil
}

func isPatched(cfgMap *corev1.ConfigMap) bool {
	var patched bool
	if len(cfgMap.ObjectMeta.Labels) > 0 {
		_, patched = cfgMap.ObjectMeta.Labels["maesh-patched"]
	}

	return patched
}

func (p *Prepare) restartPods(deployment *appsv1.Deployment) error {
	p.log.Infof("Restarting %q pods", deployment.Name)

	// Never edit original object, always work with a clone for updates.
	newDeployment := deployment.DeepCopy()
	annotations := newDeployment.Spec.Template.Annotations

	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	newDeployment.Spec.Template.Annotations = annotations
	_, err := p.client.GetKubernetesClient().AppsV1().Deployments(newDeployment.Namespace).Update(newDeployment)

	return err
}
