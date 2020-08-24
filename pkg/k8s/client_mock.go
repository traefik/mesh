package k8s

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha3"
	accessclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	fakeaccessclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned/fake"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	accesslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha2"
	specsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	fakespecsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/fake"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	specslister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha3"
	splitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	fakesplitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned/fake"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	splitlister "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubeclient "k8s.io/client-go/kubernetes"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	listers "k8s.io/client-go/listers/core/v1"
)

// Ensure the client mock fits the Client interface.
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
}

// ClientMock holds mock client.
type ClientMock struct {
	testingT *testing.T

	kubeClient   *fakekubeclient.Clientset
	accessClient *fakeaccessclient.Clientset
	specsClient  *fakespecsclient.Clientset
	splitClient  *fakesplitclient.Clientset

	informerFactory       informers.SharedInformerFactory
	accessInformerFactory accessinformer.SharedInformerFactory
	specsInformerFactory  specsinformer.SharedInformerFactory
	splitInformerFactory  splitinformer.SharedInformerFactory

	PodLister            listers.PodLister
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	NamespaceLister      listers.NamespaceLister
	TrafficTargetLister  accesslister.TrafficTargetLister
	HTTPRouteGroupLister specslister.HTTPRouteGroupLister
	TCPRouteLister       specslister.TCPRouteLister
	TrafficSplitLister   splitlister.TrafficSplitLister
}

// NewClientMock create a new client mock.
func NewClientMock(testingT *testing.T, stopCh <-chan struct{}, path string, acl bool) *ClientMock {
	yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./testdata/" + path))
	if err != nil {
		panic(err)
	}

	k8sObjects := MustParseYaml(yamlContent)
	c := &ClientMock{testingT: testingT}

	c.kubeClient = fakekubeclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, CoreObjectKinds)...)
	c.splitClient = fakesplitclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, SplitObjectKinds)...)
	c.specsClient = fakespecsclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, SpecsObjectKinds)...)
	c.specsInformerFactory = specsinformer.NewSharedInformerFactory(c.specsClient, 0)

	c.informerFactory = informers.NewSharedInformerFactory(c.kubeClient, 0)
	c.splitInformerFactory = splitinformer.NewSharedInformerFactory(c.splitClient, 0)

	c.PodLister = c.informerFactory.Core().V1().Pods().Lister()
	c.ServiceLister = c.informerFactory.Core().V1().Services().Lister()
	c.EndpointsLister = c.informerFactory.Core().V1().Endpoints().Lister()
	c.NamespaceLister = c.informerFactory.Core().V1().Namespaces().Lister()
	c.TrafficSplitLister = c.splitInformerFactory.Split().V1alpha3().TrafficSplits().Lister()
	c.HTTPRouteGroupLister = c.specsInformerFactory.Specs().V1alpha3().HTTPRouteGroups().Lister()
	c.TCPRouteLister = c.specsInformerFactory.Specs().V1alpha3().TCPRoutes().Lister()

	// Start the informers.
	c.startInformers(stopCh)

	if acl {
		c.accessClient = fakeaccessclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, AccessObjectKinds)...)

		c.accessInformerFactory = accessinformer.NewSharedInformerFactory(c.accessClient, 0)

		c.TrafficTargetLister = c.accessInformerFactory.Access().V1alpha2().TrafficTargets().Lister()

		// Start the informers.
		c.startACLInformers(stopCh)
	}

	return c
}

// startInformers waits for the kubernetes core informers to start and sync.
func (c *ClientMock) startInformers(stopCh <-chan struct{}) {
	c.informerFactory.Start(stopCh)

	for t, ok := range c.informerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			c.testingT.Logf("timed out waiting for controller caches to sync: %s", t)
		}
	}

	c.splitInformerFactory.Start(stopCh)

	for t, ok := range c.splitInformerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			c.testingT.Logf("timed out waiting for controller caches to sync: %s", t)
		}
	}

	c.specsInformerFactory.Start(stopCh)

	for t, ok := range c.specsInformerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			c.testingT.Logf("timed out waiting for controller caches to sync: %s", t)
		}
	}
}

// startACLInformers waits for the ACL informers to start and sync.
func (c *ClientMock) startACLInformers(stopCh <-chan struct{}) {
	c.accessInformerFactory.Start(stopCh)

	for t, ok := range c.accessInformerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			c.testingT.Logf("timed out waiting for controller caches to sync: %s", t)
		}
	}
}

// KubernetesClient is used to get the kubernetes clientset.
func (c *ClientMock) KubernetesClient() kubeclient.Interface {
	return c.kubeClient
}

// AccessClient is used to get the SMI Access clientset.
func (c *ClientMock) AccessClient() accessclient.Interface {
	return c.accessClient
}

// SpecsClient is used to get the SMI Specs clientset.
func (c *ClientMock) SpecsClient() specsclient.Interface {
	return c.specsClient
}

// SplitClient is used to get the SMI Split clientset.
func (c *ClientMock) SplitClient() splitclient.Interface {
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
