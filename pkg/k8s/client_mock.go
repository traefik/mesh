package k8s

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/containous/traefik/v2/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	access "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specs "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	split "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha2"
	accessClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	fakeAccessClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned/fake"
	accessInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	accessLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specsClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	fakeSpecsClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/fake"
	specsInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	specsLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	fakeSplitClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned/fake"
	splitInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	splitLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	fakeKubeClient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// Ensure the client mock fits the Client interface
var _ Client = (*ClientMock)(nil)

func init() {
	// required by k8s.MustParseYaml
	err := access.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = specs.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = split.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = v1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
}

// ClientMock holds mock client.
type ClientMock struct {
	kubeClient   *fakeKubeClient.Clientset
	accessClient *fakeAccessClient.Clientset
	specsClient  *fakeSpecsClient.Clientset
	splitClient  *fakeSplitClient.Clientset

	informerFactory       informers.SharedInformerFactory
	accessInformerFactory accessInformer.SharedInformerFactory
	specsInformerFactory  specsInformer.SharedInformerFactory
	splitInformerFactory  splitInformer.SharedInformerFactory

	PodLister            listers.PodLister
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	NamespaceLister      listers.NamespaceLister
	TrafficTargetLister  accessLister.TrafficTargetLister
	HTTPRouteGroupLister specsLister.HTTPRouteGroupLister
	TCPRouteLister       specsLister.TCPRouteLister
	TrafficSplitLister   splitLister.TrafficSplitLister
}

// NewClientMock create a new client mock.
func NewClientMock(stopCh <-chan struct{}, path string, smi bool) *ClientMock {
	yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./fixtures/" + path))
	if err != nil {
		panic(err)
	}

	k8sObjects := MustParseYaml(yamlContent)
	c := &ClientMock{}

	c.kubeClient = fakeKubeClient.NewSimpleClientset(filterObjectsByKind(k8sObjects, CoreObjectKinds)...)

	c.informerFactory = informers.NewSharedInformerFactory(c.kubeClient, 0)

	podInformer := c.informerFactory.Core().V1().Pods().Informer()
	serviceInformer := c.informerFactory.Core().V1().Services().Informer()
	endpointsInformer := c.informerFactory.Core().V1().Endpoints().Informer()
	namespaceInformer := c.informerFactory.Core().V1().Namespaces().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})
	serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})
	endpointsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})
	namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})

	c.PodLister = c.informerFactory.Core().V1().Pods().Lister()
	c.ServiceLister = c.informerFactory.Core().V1().Services().Lister()
	c.EndpointsLister = c.informerFactory.Core().V1().Endpoints().Lister()
	c.NamespaceLister = c.informerFactory.Core().V1().Namespaces().Lister()

	// Start the informers.
	c.informerFactory.Start(stopCh)

	for t, ok := range c.informerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			fmt.Printf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if smi {
		c.accessClient = fakeAccessClient.NewSimpleClientset(filterObjectsByKind(k8sObjects, AccessObjectKinds)...)
		c.specsClient = fakeSpecsClient.NewSimpleClientset(filterObjectsByKind(k8sObjects, SpecsObjectKinds)...)
		c.splitClient = fakeSplitClient.NewSimpleClientset(filterObjectsByKind(k8sObjects, SplitObjectKinds)...)

		c.accessInformerFactory = accessInformer.NewSharedInformerFactory(c.accessClient, 0)
		c.specsInformerFactory = specsInformer.NewSharedInformerFactory(c.specsClient, 0)
		c.splitInformerFactory = splitInformer.NewSharedInformerFactory(c.splitClient, 0)

		trafficTargetInformer := c.accessInformerFactory.Access().V1alpha1().TrafficTargets().Informer()
		httpRouteGroupInformer := c.specsInformerFactory.Specs().V1alpha1().HTTPRouteGroups().Informer()
		tcpRouteInformer := c.specsInformerFactory.Specs().V1alpha1().TCPRoutes().Informer()
		trafficSplitInformer := c.splitInformerFactory.Split().V1alpha2().TrafficSplits().Informer()

		trafficTargetInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})
		httpRouteGroupInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})
		tcpRouteInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})
		trafficSplitInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})

		c.TrafficTargetLister = c.accessInformerFactory.Access().V1alpha1().TrafficTargets().Lister()
		c.HTTPRouteGroupLister = c.specsInformerFactory.Specs().V1alpha1().HTTPRouteGroups().Lister()
		c.TCPRouteLister = c.specsInformerFactory.Specs().V1alpha1().TCPRoutes().Lister()
		c.TrafficSplitLister = c.splitInformerFactory.Split().V1alpha2().TrafficSplits().Lister()

		// Start the informers.
		c.accessInformerFactory.Start(stopCh)
		c.specsInformerFactory.Start(stopCh)
		c.splitInformerFactory.Start(stopCh)

		for t, ok := range c.accessInformerFactory.WaitForCacheSync(stopCh) {
			if !ok {
				fmt.Printf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		for t, ok := range c.specsInformerFactory.WaitForCacheSync(stopCh) {
			if !ok {
				fmt.Printf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		for t, ok := range c.splitInformerFactory.WaitForCacheSync(stopCh) {
			if !ok {
				fmt.Printf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}

	return c
}

// GetKubernetesClient is used to get the kubernetes clientset.
func (c *ClientMock) GetKubernetesClient() kubeClient.Interface {
	return c.kubeClient
}

// GetAccessClient is used to get the SMI Access clientset.
func (c *ClientMock) GetAccessClient() accessClient.Interface {
	return c.accessClient
}

// GetSpecsClient is used to get the SMI Specs clientset.
func (c *ClientMock) GetSpecsClient() specsClient.Interface {
	return c.specsClient
}

// GetSplitClient is used to get the SMI Split clientset.
func (c *ClientMock) GetSplitClient() splitClient.Interface {
	return c.splitClient
}

// MustParseYaml parses a YAML to objects.
func MustParseYaml(content []byte) []runtime.Object {
	acceptedK8sTypes := regexp.MustCompile(`(` + strings.Join([]string{CoreObjectKinds, AccessObjectKinds, SpecsObjectKinds, SplitObjectKinds}, "|") + `)`)

	files := strings.Split(string(content), "---")
	retVal := make([]runtime.Object, 0, len(files))

	for _, file := range files {
		if file == "\n" || file == "" {
			continue
		}

		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, groupVersionKind, err := decode([]byte(file), nil, nil)

		if err != nil {
			panic(fmt.Sprintf("Error while decoding YAML object. Err was: %s", err))
		}

		if !acceptedK8sTypes.MatchString(groupVersionKind.Kind) {
			panic(fmt.Sprintf("The custom-roles configMap contained K8s object types which are not supported! Skipping object with type: %s", groupVersionKind.Kind))
		} else {
			retVal = append(retVal, obj)
		}
	}

	return retVal
}

// filterObjectsByKind filters out objects that are not the selected kind.
func filterObjectsByKind(objects []runtime.Object, filter string) []runtime.Object {
	var result []runtime.Object

	kinds := strings.Split(filter, "|")

	for _, item := range objects {
		if contains(kinds, item.GetObjectKind().GroupVersionKind().Kind) {
			result = append(result, item)
		}
	}

	return result
}
