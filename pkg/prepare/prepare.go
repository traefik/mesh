package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/safe"
	accessInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	specsInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	splitInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

var (
	supportedCoreDNSVersions = []string{
		"1.3",
		"1.4",
		"1.5",
		"1.6",
	}
)

// Interface is an interface for the prepare methods.
type Interface interface {
	CheckCluster() error
	CheckInformersStart(smi bool) error
	InitCluster(namespace string, clusterDomain string) error
}

// Ensure the client wrapper fits the Client interface
var _ Interface = (*Prepare)(nil)

// Prepare holds the clients for the various resource controllers.
type Prepare struct {
	client k8s.Client
}

// NewPrepare returns an initialized prepare object.
func NewPrepare(client k8s.Client) Interface {
	return &Prepare{
		client: client,
	}
}

// CheckCluster is used to check the cluster.
func (p *Prepare) CheckCluster() error {
	log.Infoln("Checking Cluster...")

	match, err := p.coreDNSMatch()
	if err != nil {
		return err
	}

	if !match {
		match, err = p.kubeDNSMatch()
		if err != nil {
			return err
		}
	}

	if !match {
		return fmt.Errorf("no core dns service available for installing maesh: %v", err)
	}

	return nil
}

// coreDNSMatch checks if CoreDNS service can match.
func (p *Prepare) coreDNSMatch() (bool, error) {
	log.Infoln("Checking CoreDNS...")
	log.Debugln("Get CoreDNS version...")

	deployment, err := p.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
	exists, err := k8s.TranslateNotFoundError(err)

	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namespace %q: %v", "coredns", metav1.NamespaceSystem, err)
	}

	if !exists {
		log.Debugf("%s does not exist in namespace %s", "coredns", metav1.NamespaceSystem)
		return false, nil
	}

	var version string

	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name != "coredns" {
			continue
		}

		split := strings.Split(c.Image, ":")
		if len(split) == 2 {
			version = split[1]
		}
	}

	if !isCoreDNSVersionSupported(version) {
		return false, fmt.Errorf("unsupported CoreDNS version %q, (supported versions are: %s)", version, strings.Join(supportedCoreDNSVersions, ","))
	}

	log.Info("CoreDNS match")

	return true, nil
}

// kubeDNSMatch checks if KubeDNS service can match.
func (p *Prepare) kubeDNSMatch() (bool, error) {
	log.Infoln("Checking KubeDNS...")
	log.Debugln("Get KubeDNS version...")

	_, err := p.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	exists, err := k8s.TranslateNotFoundError(err)

	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %v", "kube-dns", metav1.NamespaceSystem, err)
	}

	if !exists {
		log.Debugf("%s does not exist in namespace %s", "kube-dns", metav1.NamespaceSystem)
		return false, nil
	}

	log.Info("KubeDNS match")

	return true, nil
}

// CheckInformersStart checks if the required informers can start and sync in a reasonable time.
func (p *Prepare) CheckInformersStart(smi bool) error {
	log.Debug("Creating and Starting Informers")

	stopCh := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a new SharedInformerFactory, and register the event handler to informers.
	kubeFactory := informers.NewSharedInformerFactoryWithOptions(p.client.GetKubernetesClient(), k8s.ResyncPeriod)
	kubeFactory.Core().V1().Services().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Core().V1().Endpoints().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Core().V1().Pods().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Start(stopCh)

	for t, ok := range kubeFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if smi {
		// Create new SharedInformerFactories, and register the event handler to informers.
		accessFactory := accessInformer.NewSharedInformerFactoryWithOptions(p.client.GetAccessClient(), k8s.ResyncPeriod)
		accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		accessFactory.Start(stopCh)

		for t, ok := range accessFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		specsFactory := specsInformer.NewSharedInformerFactoryWithOptions(p.client.GetSpecsClient(), k8s.ResyncPeriod)
		specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		specsFactory.Start(stopCh)

		for t, ok := range specsFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		splitFactory := splitInformer.NewSharedInformerFactoryWithOptions(p.client.GetSplitClient(), k8s.ResyncPeriod)
		splitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		splitFactory.Start(stopCh)

		for t, ok := range splitFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}

	return nil
}

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options.
func (p *Prepare) InitCluster(namespace string, clusterDomain string) error {
	log.Infoln("Preparing Cluster...")
	log.Debugln("Patching DNS...")

	if err := p.patchDNS(metav1.NamespaceSystem, clusterDomain, namespace); err != nil {
		return err
	}

	log.Infoln("Cluster Preparation Complete...")

	return nil
}

// patchDNS is used to patch the CoreDNS configmap if needed.
func (p *Prepare) patchDNS(coreNamespace, clusterDomain, maeshNamespace string) error {
	deployment, err := p.client.GetKubernetesClient().AppsV1().Deployments(coreNamespace).Get("coredns", metav1.GetOptions{})
	exists, err := k8s.TranslateNotFoundError(err)

	if err != nil {
		return err
	}

	// If CoreDNS exist we will patch it.
	if exists {
		log.Debugln("Patching CoreDNS configmap...")

		var patched bool

		patched, err = p.patchCoreDNSConfigMap(deployment, clusterDomain, maeshNamespace)
		if err != nil {
			return err
		}

		if !patched {
			log.Debugln("Restarting CoreDNS pods...")

			if err = p.restartPods(deployment); err != nil {
				return err
			}

			return nil
		}

		return nil
	}

	log.Debugln("coredns not available fallback to kube-dns")
	// If coreDNS does not exist we try to get the kube-dns
	deployment, err = p.client.GetKubernetesClient().AppsV1().Deployments(coreNamespace).Get("kube-dns", metav1.GetOptions{})
	exists, err = k8s.TranslateNotFoundError(err)

	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("neither CoreDNS or KubeDNS are available in namespace %q", coreNamespace)
	}

	ebo := backoff.NewConstantBackOff(10 * time.Second)

	var serviceIP string

	log.Debugln("Get CoreDNS service IP")

	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		svc, errSvc := p.client.GetKubernetesClient().CoreV1().Services("maesh").Get("coredns", metav1.GetOptions{})
		exists, errSvc := k8s.TranslateNotFoundError(errSvc)
		if errSvc != nil {
			return fmt.Errorf("unable get the service %q in namespace %q: %v", "coredns", "maesh", errSvc)
		}
		if !exists {
			return fmt.Errorf("service %q has not been yet created", "coredns")
		}
		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("service %q has no clusterIP", "coredns")
		}

		serviceIP = svc.Spec.ClusterIP
		return nil
	}), ebo); err != nil {
		return fmt.Errorf("unable get the service %q in namespace %q: %v", "coredns", "maesh", err)
	}

	// Patch KubeDNS
	log.Debugln("Patching KubeDNS configmap... with IP: ", serviceIP)

	patched, err := p.patchKubeDNSConfigMap(deployment, serviceIP)
	if err != nil {
		return err
	}

	if !patched {
		log.Debugln("Restarting KubeDNS pods...")

		if err := p.restartPods(deployment); err != nil {
			return err
		}
	}

	return nil
}

func (p *Prepare) patchCoreDNSConfigMap(coreDeployment *appsv1.Deployment, clusterDomain, maeshNamespace string) (bool, error) {
	var coreConfigMapName string

	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName = getCoreDNSConfigMapName(coreDeployment)

	coreConfigMap, err := p.client.GetKubernetesClient().CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(coreConfigMap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigMap.ObjectMeta.Labels["maesh-patched"]; ok {
			log.Debugln("Configmap already patched...")
			return true, nil
		}
	}

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

	if len(coreConfigMap.ObjectMeta.Labels) == 0 {
		coreConfigMap.ObjectMeta.Labels = make(map[string]string)
	}

	coreConfigMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err = p.client.GetKubernetesClient().CoreV1().ConfigMaps(coreDeployment.Namespace).Update(coreConfigMap); err != nil {
		return false, err
	}

	return false, nil
}

func (p *Prepare) patchKubeDNSConfigMap(deployment *appsv1.Deployment, coreDNSIp string) (bool, error) {
	var configMapName string

	if len(deployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("kube-dns configmap not defined")
	}

	configMapName = deployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	configMap, err := p.client.GetKubernetesClient().CoreV1().ConfigMaps(deployment.Namespace).Get(configMapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(configMap.ObjectMeta.Labels) > 0 {
		if _, ok := configMap.ObjectMeta.Labels["maesh-patched"]; ok {
			log.Debugln("Configmap already patched...")
			return true, nil
		}
	}

	stubDomains := make(map[string][]string)
	originalBlock, exist := configMap.Data["stubDomains"]

	if !exist {
		originalBlock = "{}"
	}

	if err = json.Unmarshal([]byte(originalBlock), &stubDomains); err != nil {
		return false, err
	}

	stubDomains["maesh"] = []string{coreDNSIp}

	var newData []byte

	newData, err = json.Marshal(stubDomains)
	if err != nil {
		return false, err
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	configMap.Data["stubDomains"] = string(newData)

	if len(configMap.ObjectMeta.Labels) == 0 {
		configMap.ObjectMeta.Labels = make(map[string]string)
	}

	configMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err = p.client.GetKubernetesClient().CoreV1().ConfigMaps(deployment.Namespace).Update(configMap); err != nil {
		return false, err
	}

	return false, nil
}

func (p *Prepare) restartPods(deployment *appsv1.Deployment) error {
	log.Infof("Restarting %s pods...\n", deployment.Name)

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

// getCoreDNSConfigMapName returns the dected coredns configmap name
func getCoreDNSConfigMapName(coreDeployment *appsv1.Deployment) string {
	for _, volume := range coreDeployment.Spec.Template.Spec.Volumes {
		if volume.ConfigMap != nil {
			return volume.ConfigMap.Name
		}
	}

	return ""
}

// isCoreDNSVersionSupported returns true if the provided string contains a supported CoreDNS version.
func isCoreDNSVersionSupported(versionLine string) bool {
	for _, v := range supportedCoreDNSVersions {
		if strings.Contains(versionLine, v) || strings.Contains(versionLine, "v"+v) {
			return true
		}
	}

	return false
}
