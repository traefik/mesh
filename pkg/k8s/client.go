package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/traefik/v2/pkg/safe"

	accessClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	accessInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	specsClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	specsInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	splitClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	splitInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	supportedCoreDNSVersions = []string{
		"1.3",
		"1.4",
		"1.5",
		"1.6",
	}
)

// Client is an interface for the various resource controllers.
type Client interface {
	GetKubernetesClient() kubeClient.Interface
	GetAccessClient() accessClient.Interface
	GetSpecsClient() specsClient.Interface
	GetSplitClient() splitClient.Interface

	CreateService(service *corev1.Service) (*corev1.Service, error)
	DeleteService(namespace, name string) error
	UpdateService(service *corev1.Service) (*corev1.Service, error)
}

// Ensure the client wrapper fits the Client interface
var _ Client = (*ClientWrapper)(nil)

// ClientWrapper holds the clients for the various resource controllers.
type ClientWrapper struct {
	kubeClient   *kubeClient.Clientset
	accessClient *accessClient.Clientset
	specsClient  *specsClient.Clientset
	splitClient  *splitClient.Clientset
}

// NewClientWrapper creates and returns both a kubernetes client, and a CRD client.
func NewClientWrapper(url string, kubeConfig string) (*ClientWrapper, error) {
	config, err := clientcmd.BuildConfigFromFlags(url, kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := buildKubernetesClient(config)
	if err != nil {
		return nil, err
	}

	accessClient, err := buildSmiAccessClient(config)
	if err != nil {
		return nil, err
	}

	specsClient, err := buildSmiSpecsClient(config)
	if err != nil {
		return nil, err
	}

	splitClient, err := buildSmiSplitClient(config)
	if err != nil {
		return nil, err
	}

	return &ClientWrapper{
		kubeClient:   kubeClient,
		accessClient: accessClient,
		specsClient:  specsClient,
		splitClient:  splitClient,
	}, nil
}

// CheckCluster is used to check the cluster.
func (w *ClientWrapper) CheckCluster() error {
	log.Infoln("Checking Cluster...")

	match, err := w.CoreDNSMatch()
	if err != nil {
		return err
	}

	if !match {
		match, err = w.KubeDNSMatch()
		if err != nil {
			return err
		}
	}

	if !match {
		return fmt.Errorf("no core dns service available for installing maesh: %v", err)
	}

	return nil
}

// GetKubernetesClient is used to get the kubernetes clientset.
func (w *ClientWrapper) GetKubernetesClient() kubeClient.Interface {
	return w.kubeClient
}

// GetAccessClient is used to get the SMI Access clientset.
func (w *ClientWrapper) GetAccessClient() accessClient.Interface {
	return w.accessClient
}

// GetSpecsClient is used to get the SMI Specs clientset.
func (w *ClientWrapper) GetSpecsClient() specsClient.Interface {
	return w.specsClient
}

// GetSplitClient is used to get the SMI Split clientset.
func (w *ClientWrapper) GetSplitClient() splitClient.Interface {
	return w.splitClient
}

// CoreDNSMatch checks if CoreDNS service can match.
func (w *ClientWrapper) CoreDNSMatch() (bool, error) {
	log.Infoln("Checking CoreDNS...")
	log.Debugln("Get CoreDNS version...")

	deployment, exists, err := w.GetDeployment(metav1.NamespaceSystem, "coredns")
	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %v", "coredns", metav1.NamespaceSystem, err)
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

// KubeDNSMatch checks if KubeDNS service can match.
func (w *ClientWrapper) KubeDNSMatch() (bool, error) {
	log.Infoln("Checking KubeDNS...")
	log.Debugln("Get KubeDNS version...")

	_, exists, err := w.GetDeployment(metav1.NamespaceSystem, "kube-dns")
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
func (w *ClientWrapper) CheckInformersStart(smi bool) error {
	log.Debug("Creating and Starting Informers")

	stopCh := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a new SharedInformerFactory, and register the event handler to informers.
	kubeFactory := informers.NewSharedInformerFactoryWithOptions(w.kubeClient, ResyncPeriod)
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
		accessFactory := accessInformer.NewSharedInformerFactoryWithOptions(w.accessClient, ResyncPeriod)
		accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		accessFactory.Start(stopCh)

		for t, ok := range accessFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		specsFactory := specsInformer.NewSharedInformerFactoryWithOptions(w.specsClient, ResyncPeriod)
		specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
		specsFactory.Start(stopCh)

		for t, ok := range specsFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		splitFactory := splitInformer.NewSharedInformerFactoryWithOptions(w.splitClient, ResyncPeriod)
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

// isCoreDNSVersionSupported returns true if the provided string contains a supported CoreDNS version.
func isCoreDNSVersionSupported(versionLine string) bool {
	for _, v := range supportedCoreDNSVersions {
		if strings.Contains(versionLine, v) || strings.Contains(versionLine, "v"+v) {
			return true
		}
	}

	return false
}

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options.
func (w *ClientWrapper) InitCluster(namespace string, clusterDomain string) error {
	log.Infoln("Preparing Cluster...")
	log.Debugln("Patching DNS...")

	if err := w.patchDNS(metav1.NamespaceSystem, clusterDomain, namespace); err != nil {
		return err
	}

	log.Infoln("Cluster Preparation Complete...")

	return nil
}

func (w *ClientWrapper) patchDNS(coreNamespace, clusterDomain, maeshNamespace string) error {
	deployment, exist, err := w.GetDeployment(coreNamespace, "coredns")
	if err != nil {
		return err
	}

	// If CoreDNS exist we will patch it.
	if exist {
		log.Debugln("Patching CoreDNS configmap...")

		var patched bool

		patched, err = w.patchCoreDNSConfigMap(deployment, clusterDomain, maeshNamespace)
		if err != nil {
			return err
		}

		if !patched {
			log.Debugln("Restarting CoreDNS pods...")

			if err = w.restartPods(deployment); err != nil {
				return err
			}

			return nil
		}

		return nil
	}

	log.Debugln("coredns not available fallback to kube-dns")
	// If coreDNS does not exist we try to get the kube-dns
	deployment, exist, err = w.GetDeployment(coreNamespace, "kube-dns")
	if err != nil {
		return err
	}

	if !exist {
		return fmt.Errorf("neither CoreDNS or KubeDNS are available in namespace %q", coreNamespace)
	}

	ebo := backoff.NewConstantBackOff(10 * time.Second)

	var serviceIP string

	log.Debugln("Get CoreDNS service IP")

	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		svc, exists, errSvc := w.GetService("maesh", "coredns")
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

	patched, err := w.patchKubeDNSConfigMap(deployment, serviceIP)
	if err != nil {
		return err
	}

	if !patched {
		log.Debugln("Restarting KubeDNS pods...")

		if err := w.restartPods(deployment); err != nil {
			return err
		}
	}

	return nil
}

func (w *ClientWrapper) patchCoreDNSConfigMap(coreDeployment *appsv1.Deployment, clusterDomain, maeshNamespace string) (bool, error) {
	var coreConfigMapName string

	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName = getCoreDNSConfigMapName(coreDeployment)

	coreConfigMap, err := w.kubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
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

	if _, err = w.kubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Update(coreConfigMap); err != nil {
		return false, err
	}

	return false, nil
}

func (w *ClientWrapper) patchKubeDNSConfigMap(deployment *appsv1.Deployment, coreDNSIp string) (bool, error) {
	var configMapName string

	if len(deployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("kube-dns configmap not defined")
	}

	configMapName = deployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	configMap, err := w.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Get(configMapName, metav1.GetOptions{})
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

	if _, err = w.kubeClient.CoreV1().ConfigMaps(deployment.Namespace).Update(configMap); err != nil {
		return false, err
	}

	return false, nil
}

func (w *ClientWrapper) restartPods(deployment *appsv1.Deployment) error {
	log.Infof("Restarting %s pods...\n", deployment.Name)

	// Never edit original object, always work with a clone for updates.
	newDeployment := deployment.DeepCopy()
	annotations := newDeployment.Spec.Template.Annotations

	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	newDeployment.Spec.Template.Annotations = annotations
	_, err := w.kubeClient.AppsV1().Deployments(newDeployment.Namespace).Update(newDeployment)

	return err
}

// VerifyCluster is used to verify a kubernetes cluster has been initialized properly.
func (w *ClientWrapper) VerifyCluster() error {
	log.Infoln("Verifying Cluster...")
	defer log.Infoln("Cluster Verification Complete...")
	log.Debugln("Verifying CoreDNS Patched...")

	if err := w.isCoreDNSPatched("coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	return nil
}

func (w *ClientWrapper) isCoreDNSPatched(deploymentName string, namespace string) error {
	coreDeployment, err := w.kubeClient.AppsV1().Deployments(namespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName := getCoreDNSConfigMapName(coreDeployment)

	coreConfigMap, err := w.kubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreConfigMap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigMap.ObjectMeta.Labels["maesh-patched"]; ok {
			return nil
		}
	}

	return errors.New("coreDNS not patched. Run ./maesh patch to update DNS")
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(config *rest.Config) (*kubeClient.Clientset, error) {
	log.Debugln("Building Kubernetes Client...")

	client, err := kubeClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	return client, nil
}

// buildSmiAccessClient returns a client to manage SMI Access objects.
func buildSmiAccessClient(config *rest.Config) (*accessClient.Clientset, error) {
	log.Debugln("Building SMI Access Client...")

	client, err := accessClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Access Client: %v", err)
	}

	return client, nil
}

// buildSmiSpecsClient returns a client to manage SMI Specs objects.
func buildSmiSpecsClient(config *rest.Config) (*specsClient.Clientset, error) {
	log.Debugln("Building SMI Specs Client...")

	client, err := specsClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Specs Client: %v", err)
	}

	return client, nil
}

// buildSmiSplitClient returns a client to manage SMI Split objects.
func buildSmiSplitClient(config *rest.Config) (*splitClient.Clientset, error) {
	log.Debugln("Building SMI Split Client...")

	client, err := splitClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Split Client: %v", err)
	}

	return client, nil
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

// GetService retrieves the service from the specified namespace.
func (w *ClientWrapper) GetService(namespace, name string) (*corev1.Service, bool, error) {
	service, err := w.kubeClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)

	return service, exists, err
}

// DeleteService deletes the service from the specified namespace.
func (w *ClientWrapper) DeleteService(namespace, name string) error {
	return w.kubeClient.CoreV1().Services(namespace).Delete(name, &metav1.DeleteOptions{})
}

// CreateService create the specified service.
func (w *ClientWrapper) CreateService(service *corev1.Service) (*corev1.Service, error) {
	return w.kubeClient.CoreV1().Services(service.Namespace).Create(service)
}

// UpdateService updates the specified service.
func (w *ClientWrapper) UpdateService(service *corev1.Service) (*corev1.Service, error) {
	return w.kubeClient.CoreV1().Services(service.Namespace).Update(service)
}

// ListPodWithOptions retrieves pods from the specified namespace.
func (w *ClientWrapper) ListPodWithOptions(namespace string, options metav1.ListOptions) (*corev1.PodList, error) {
	return w.kubeClient.CoreV1().Pods(namespace).List(options)
}

// GetNamespace returns a namespace.
func (w *ClientWrapper) GetNamespace(name string) (*corev1.Namespace, bool, error) {
	pod, err := w.kubeClient.CoreV1().Namespaces().Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)

	return pod, exists, err
}

// GetDeployment retrieves the deployment from the specified namespace.
func (w *ClientWrapper) GetDeployment(namespace, name string) (*appsv1.Deployment, bool, error) {
	deployment, err := w.kubeClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)

	return deployment, exists, err
}

// UpdateDeployment updates the specified deployment.
func (w *ClientWrapper) UpdateDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return w.kubeClient.AppsV1().Deployments(deployment.Namespace).Update(deployment)
}

// UpdateConfigMap updates the specified configMap.
func (w *ClientWrapper) UpdateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return w.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
}

// CreateConfigMap creates the specified configMap.
func (w *ClientWrapper) CreateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return w.kubeClient.CoreV1().ConfigMaps(configMap.Namespace).Create(configMap)
}

// translateNotFoundError will translate a "not found" error to a boolean return
// value which indicates if the resource exists and a nil error.
func translateNotFoundError(err error) (bool, error) {
	if kubeerror.IsNotFound(err) {
		return false, nil
	}

	return err == nil, err
}
