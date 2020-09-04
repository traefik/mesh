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
	specsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	fakespecsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/fake"
	splitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	fakesplitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/runtime"
	kubeclient "k8s.io/client-go/kubernetes"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
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
}

// NewClientMock create a new client mock.
func NewClientMock(testingT *testing.T, path string) *ClientMock {
	yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./testdata/" + path))
	if err != nil {
		panic(err)
	}

	k8sObjects := MustParseYaml(yamlContent)
	c := &ClientMock{testingT: testingT}

	c.kubeClient = fakekubeclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, CoreObjectKinds)...)
	c.splitClient = fakesplitclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, SplitObjectKinds)...)
	c.specsClient = fakespecsclient.NewSimpleClientset(filterObjectsByKind(k8sObjects, SpecsObjectKinds)...)

	return c
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
