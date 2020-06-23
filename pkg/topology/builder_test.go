package topology

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	mk8s "github.com/containous/maesh/pkg/k8s"
	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha1"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha1"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	accessclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	accessfake "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned/fake"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	specsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	specfake "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/fake"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	splitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	splitfake "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned/fake"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// TestTopologyBuilder_BuildIgnoresNamespaces makes sure namespace to ignore are ignored by the TopologyBuilder.
func TestTopologyBuilder_BuildIgnoresNamespaces(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	annotations := map[string]string{
		"maesh.containo.us/traffic-type":      "http",
		"maesh.containo.us/ratelimit-average": "100",
		"maesh.containo.us/ratelimit-burst":   "200",
	}
	svcbPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}
	svccPorts := []corev1.ServicePort{svcPort("port-9091", 9091, 9091)}
	svcdPorts := []corev1.ServicePort{svcPort("port-9092", 9092, 9092)}

	saA := createServiceAccount("ignored-ns", "service-account-a")
	podA := createPod("ignored-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("ignored-ns", "service-account-b")
	svcB := createService("ignored-ns", "svc-b", annotations, svcbPorts, selectorAppB, "10.10.1.16")
	podB := createPod("ignored-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	svcC := createService("ignored-ns", "svc-c", annotations, svccPorts, selectorAppA, "10.10.1.17")
	svcD := createService("ignored-ns", "svc-d", annotations, svcdPorts, selectorAppA, "10.10.1.18")

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("ignored-ns", "http-rt-grp-ignored", []spec.HTTPMatch{apiMatch, metricMatch})

	tt := createTrafficTarget("ignored-ns", "tt", saB, "8080", []*corev1.ServiceAccount{saA}, rtGrp, []string{})
	ts := createTrafficSplit("ignored-ns", "ts", svcB, svcC, svcD)

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, svcB, podB, svcC, svcD)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset(ts)
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter(mk8s.IgnoreNamespaces("ignored-ns"))

	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	want := &Topology{
		Services:              make(map[Key]*Service),
		Pods:                  make(map[Key]*Pod),
		ServiceTrafficTargets: make(map[ServiceTrafficTargetKey]*ServiceTrafficTarget),
		TrafficSplits:         make(map[Key]*TrafficSplit),
	}

	assert.Equal(t, want, got)
}

func TestTopologyBuilder_HandleCircularReferenceOnTrafficSplit(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	selectorAppC := map[string]string{"app": "app-c"}
	selectorAppD := map[string]string{"app": "app-d"}
	selectorAppE := map[string]string{"app": "app-e"}
	annotations := map[string]string{}
	svcPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA1 := createServiceAccount("my-ns", "service-account-a")
	saA2 := createServiceAccount("my-ns", "service-account-a-2")
	podA1 := createPod("my-ns", "app-a", saA1, selectorAppA, "10.10.1.1")
	podA2 := createPod("my-ns", "app-a-2", saA2, selectorAppA, "10.10.1.2")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	saC := createServiceAccount("my-ns", "service-account-c")
	svcC := createService("my-ns", "svc-c", annotations, svcPorts, selectorAppC, "10.10.1.17")
	podC := createPod("my-ns", "app-c", saC, svcC.Spec.Selector, "10.10.2.2")

	saD := createServiceAccount("my-ns", "service-account-d")
	svcD := createService("my-ns", "svc-d", annotations, svcPorts, selectorAppD, "10.10.1.18")
	podD := createPod("my-ns", "app-d", saD, svcD.Spec.Selector, "10.10.2.3")

	saE := createServiceAccount("my-ns", "service-account-e")
	svcE := createService("my-ns", "svc-e", annotations, svcPorts, selectorAppE, "10.10.1.19")
	podE := createPod("my-ns", "app-e", saE, svcE.Spec.Selector, "10.10.2.4")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})
	epC := createEndpoints(svcC, []*corev1.Pod{podC})
	epD := createEndpoints(svcD, []*corev1.Pod{podD})
	epE := createEndpoints(svcE, []*corev1.Pod{podE})

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	ttb := createTrafficTarget("my-ns", "tt-b", saB, "8080", []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttd := createTrafficTarget("my-ns", "tt-d", saD, "8080", []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, "8080", []*corev1.ServiceAccount{saA1, saA2}, rtGrp, ttMatch)
	tte := createTrafficTarget("my-ns", "tt-e", saE, "8080", []*corev1.ServiceAccount{saA2}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD)
	tsErr := createTrafficSplit("my-ns", "tsErr", svcC, svcB, svcE)

	k8sClient := fake.NewSimpleClientset(saA1, saA2, saB, saC, saD, saE,
		podA1, podA2, podB, podC, podD, podE,
		svcB, svcC, svcD, svcE,
		epB, epC, epD, epE)
	smiAccessClient := accessfake.NewSimpleClientset(ttb, ttc, ttd, tte)
	smiSplitClient := splitfake.NewSimpleClientset(ts, tsErr)
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assert.Len(t, got.Services[nn(svcB.Name, svcB.Namespace)].TrafficSplits, 0)
	assert.Len(t, got.Services[nn(svcC.Name, svcC.Namespace)].TrafficSplits, 0)
}

func TestTopologyBuilder_TrafficTargetSourcesForbiddenTrafficSplit(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	selectorAppC := map[string]string{"app": "app-c"}
	selectorAppD := map[string]string{"app": "app-d"}
	annotations := map[string]string{
		"maesh.containo.us/traffic-type":      "http",
		"maesh.containo.us/ratelimit-average": "100",
		"maesh.containo.us/ratelimit-burst":   "200",
	}
	svcPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA := createServiceAccount("my-ns", "service-account-a")
	saA2 := createServiceAccount("my-ns", "service-account-a-2")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")
	podA2 := createPod("my-ns", "app-a-2", saA2, selectorAppA, "10.10.1.2")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	saC := createServiceAccount("my-ns", "service-account-c")
	svcC := createService("my-ns", "svc-c", annotations, svcPorts, selectorAppC, "10.10.1.17")
	podC := createPod("my-ns", "app-c", saC, svcC.Spec.Selector, "10.10.2.2")

	saD := createServiceAccount("my-ns", "service-account-d")
	svcD := createService("my-ns", "svc-d", annotations, svcPorts, selectorAppD, "10.10.1.18")
	podD := createPod("my-ns", "app-d", saD, svcD.Spec.Selector, "10.10.2.3")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})
	epC := createEndpoints(svcC, []*corev1.Pod{podC})
	epD := createEndpoints(svcD, []*corev1.Pod{podD})

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	tt := createTrafficTarget("my-ns", "tt", saB, "8080", []*corev1.ServiceAccount{saA}, rtGrp, ttMatch)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, "8080", []*corev1.ServiceAccount{saA, saA2}, rtGrp, ttMatch)
	ttd := createTrafficTarget("my-ns", "tt-d", saD, "8080", []*corev1.ServiceAccount{saC}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD)

	k8sClient := fake.NewSimpleClientset(saA, podA, podA2, saB, svcB, podB, svcC, svcD, podC, podD, epB, epC, epD)
	smiAccessClient := accessfake.NewSimpleClientset(tt, ttc, ttd)
	smiSplitClient := splitfake.NewSimpleClientset(ts)
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	svcKey := nn(svcB.Name, svcB.Namespace)
	tsKey := got.Services[svcKey].TrafficSplits[0]

	assert.Equal(t, 0, len(got.TrafficSplits[tsKey].Incoming))
}

// TestTopologyBuilder_EvaluatesIncomingTrafficSplit makes sure a topology can be built with TrafficSplits. It also
// checks that if multiple TrafficSplits are applied to the same Service, only one will be used.
func TestTopologyBuilder_EvaluatesIncomingTrafficSplit(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	selectorAppC := map[string]string{"app": "app-c"}
	selectorAppD := map[string]string{"app": "app-d"}
	selectorAppE := map[string]string{"app": "app-e"}
	annotations := map[string]string{}
	svcPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA1 := createServiceAccount("my-ns", "service-account-a")
	saA2 := createServiceAccount("my-ns", "service-account-a-2")
	podA1 := createPod("my-ns", "app-a", saA1, selectorAppA, "10.10.1.1")
	podA2 := createPod("my-ns", "app-a-2", saA2, selectorAppA, "10.10.1.2")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	saC := createServiceAccount("my-ns", "service-account-c")
	svcC := createService("my-ns", "svc-c", annotations, svcPorts, selectorAppC, "10.10.1.17")
	podC := createPod("my-ns", "app-c", saC, svcC.Spec.Selector, "10.10.2.2")

	saD := createServiceAccount("my-ns", "service-account-d")
	svcD := createService("my-ns", "svc-d", annotations, svcPorts, selectorAppD, "10.10.1.18")
	podD := createPod("my-ns", "app-d", saD, svcD.Spec.Selector, "10.10.2.3")

	saE := createServiceAccount("my-ns", "service-account-e")
	svcE := createService("my-ns", "svc-e", annotations, svcPorts, selectorAppE, "10.10.1.19")
	podE := createPod("my-ns", "app-e", saE, svcE.Spec.Selector, "10.10.2.4")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})
	epC := createEndpoints(svcC, []*corev1.Pod{podC})
	epD := createEndpoints(svcD, []*corev1.Pod{podD})
	epE := createEndpoints(svcE, []*corev1.Pod{podE})

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	ttb := createTrafficTarget("my-ns", "tt-b", saB, "8080", []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttd := createTrafficTarget("my-ns", "tt-d", saD, "8080", []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, "8080", []*corev1.ServiceAccount{saA1, saA2}, rtGrp, ttMatch)
	tte := createTrafficTarget("my-ns", "tt-e", saE, "8080", []*corev1.ServiceAccount{saA2}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD)
	ts2 := createTrafficSplit("my-ns", "ts2", svcB, svcC, svcE)

	k8sClient := fake.NewSimpleClientset(saA1, saA2, saB, saC, saD, saE,
		podA1, podA2, podB, podC, podD, podE,
		svcB, svcC, svcD, svcE,
		epB, epC, epD, epE)
	smiAccessClient := accessfake.NewSimpleClientset(ttb, ttc, ttd, tte)
	smiSplitClient := splitfake.NewSimpleClientset(ts, ts2)
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	svcKey := nn(svcB.Name, svcB.Namespace)
	tsKeys := got.Services[svcKey].TrafficSplits

	// Make sure the resulting Topology only has a single TrafficSplit.
	assert.Len(t, tsKeys, 1)
	assert.Len(t, got.TrafficSplits, 1)

	incoming := got.TrafficSplits[tsKeys[0]].Incoming

	assert.Equal(t, 1, len(incoming))

	if tsKeys[0].Name == "ts2" {
		assert.Equal(t, "10.10.1.2", got.Pods[incoming[0]].IP)
	} else {
		assert.Equal(t, "10.10.1.1", got.Pods[incoming[0]].IP)
	}
}

// TestTopologyBuilder_BuildWithTrafficTarget makes sure a topology can be built using TrafficTargets.
func TestTopologyBuilder_BuildWithTrafficTarget(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	annotations := map[string]string{}
	svcPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA := createServiceAccount("my-ns", "service-account-a")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	tt := createTrafficTarget("my-ns", "tt", saB, "8080", []*corev1.ServiceAccount{saA}, rtGrp, ttMatch)

	k8sClient := fake.NewSimpleClientset(saA, saB, podA, podB, svcB, epB)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-traffic-target.json", got)
}

// TestTopologyBuilder_BuildWithTrafficTargetAndTrafficSplitOnSameService makes sure a TrafficTarget won't be applied on
// a service if there is already a TrafficSplit applied to it.
func TestTopologyBuilder_BuildWithTrafficTargetAndTrafficSplitOnSameService(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	selectorAppC := map[string]string{"app": "app-c"}
	selectorAppD := map[string]string{"app": "app-d"}
	annotations := map[string]string{
		"maesh.containo.us/traffic-type":      "http",
		"maesh.containo.us/ratelimit-average": "100",
		"maesh.containo.us/ratelimit-burst":   "200",
	}
	svcPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA := createServiceAccount("my-ns", "service-account-a")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	saC := createServiceAccount("my-ns", "service-account-c")
	svcC := createService("my-ns", "svc-c", annotations, svcPorts, selectorAppC, "10.10.1.17")
	podC := createPod("my-ns", "app-c", saC, svcC.Spec.Selector, "10.10.2.2")

	saD := createServiceAccount("my-ns", "service-account-d")
	svcD := createService("my-ns", "svc-d", annotations, svcPorts, selectorAppD, "10.10.1.18")
	podD := createPod("my-ns", "app-d", saD, svcD.Spec.Selector, "10.10.2.3")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})
	epC := createEndpoints(svcC, []*corev1.Pod{podC})
	epD := createEndpoints(svcD, []*corev1.Pod{podD})

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	tt := createTrafficTarget("my-ns", "tt", saB, "8080", []*corev1.ServiceAccount{saA}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD)

	k8sClient := fake.NewSimpleClientset(saA, saB, saC, saD,
		podA, podB, podC, podD,
		svcB, svcC, svcD,
		epB, epC, epD)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset(ts)
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-traffic-split-traffic-target.json", got)
}

// TestTopologyBuilder_BuildWithTrafficTargetSpecEmptyMatch makes sure that when TrafficTarget.Spec.Matches is empty,
// the output list contains all the matches defined in the HTTPRouteGroup (as defined by the
// spec https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-access-control.md#traffictarget-v1alpha1)
func TestTopologyBuilder_BuildWithTrafficTargetSpecEmptyMatch(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	annotations := map[string]string{}
	svcbPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA := createServiceAccount("my-ns", "service-account-a")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcbPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	tt := createTrafficTarget("my-ns", "tt", saB, "8080", []*corev1.ServiceAccount{saA}, rtGrp, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, svcB, podB, epB)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-spec-with-empty-match.json", got)
}

// TestTopologyBuilder_BuildWithTrafficTargetEmptyDestinationPort makes sure that when a TrafficTarget.Destination.Port
// is empty, the output contains all the ports defined by the destination service (as defined by the
// spec https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-access-control.md#traffictarget-v1alpha1)
func TestTopologyBuilder_BuildWithTrafficTargetEmptyDestinationPort(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	annotations := map[string]string{
		"maesh.containo.us/traffic-type":      "http",
		"maesh.containo.us/ratelimit-average": "100",
		"maesh.containo.us/ratelimit-burst":   "200",
	}
	svcbPorts := []corev1.ServicePort{
		svcPort("port-8080", 8080, 8080),
		svcPort("port-9090", 9090, 9090),
	}

	saA := createServiceAccount("my-ns", "service-account-a")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("my-ns", "service-account-b")
	svcB := createService("my-ns", "svc-b", annotations, svcbPorts, selectorAppB, "10.10.1.16")
	podB := createPod("my-ns", "app-b", saB, svcB.Spec.Selector, "10.10.2.1")

	epB := createEndpoints(svcB, []*corev1.Pod{podB})

	tt := createTrafficTarget("my-ns", "tt", saB, "", []*corev1.ServiceAccount{saA}, nil, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, svcB, podB, epB)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specfake.NewSimpleClientset()

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-empty-destination-port.json", got)
}

func TestTopologyBuilder_BuildWithTrafficTargetAndMismatchServicePort(t *testing.T) {
	annotations := map[string]string{}

	selectorAppA := map[string]string{"app": "app-a"}
	saA := createServiceAccount("my-ns", "service-account-a")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("my-ns", "service-account-b")

	selectorAppB1 := map[string]string{"app": "app-b1"}
	svcB1Ports := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}
	podB1 := createPod("my-ns", "app-b1", saB, selectorAppB1, "10.10.1.2")
	svcB1 := createService("my-ns", "svc-b1", annotations, svcB1Ports, selectorAppB1, "10.10.1.16")
	epB1 := createEndpoints(svcB1, []*corev1.Pod{podB1})

	selectorAppB2 := map[string]string{"app": "app-b2"}
	svcB2Ports := []corev1.ServicePort{svcPort("port-80", 80, 80)}
	podB2 := createPod("my-ns", "app-b2", saB, selectorAppB2, "10.10.1.3")
	svcB2 := createService("my-ns", "svc-b2", annotations, svcB2Ports, selectorAppB2, "10.10.1.17")
	epB2 := createEndpoints(svcB2, []*corev1.Pod{podB2})

	tt := createTrafficTarget("my-ns", "tt", saB, "80", []*corev1.ServiceAccount{saA}, nil, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, podB1, svcB1, epB1, podB2, svcB2, epB2)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specfake.NewSimpleClientset()

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-traffic-target-service-port-mismatch.json", got)
}

// TestTopologyBuilder_BuildTrafficTargetMultipleSourcesAndDestinations makes sure we can build a topology with
// a TrafficTarget defined with multiple sources.
func TestTopologyBuilder_BuildTrafficTargetMultipleSourcesAndDestinations(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	selectorAppC := map[string]string{"app": "app-c"}
	annotations := map[string]string{}
	svccPorts := []corev1.ServicePort{svcPort("port-8080", 8080, 8080)}

	saA := createServiceAccount("my-ns", "service-account-a")
	podA := createPod("my-ns", "app-a", saA, selectorAppA, "10.10.1.1")

	saB := createServiceAccount("my-ns", "service-account-b")
	podB := createPod("my-ns", "app-b", saB, selectorAppB, "10.10.2.1")

	saC := createServiceAccount("my-ns", "service-account-c")
	svcC := createService("my-ns", "svc-c", annotations, svccPorts, selectorAppC, "10.10.1.16")
	podC1 := createPod("my-ns", "app-c-1", saC, svcC.Spec.Selector, "10.10.3.1")
	podC2 := createPod("my-ns", "app-c-2", saC, svcC.Spec.Selector, "10.10.3.2")

	epC := createEndpoints(svcC, []*corev1.Pod{podC1, podC2})

	tt := createTrafficTarget("my-ns", "tt", saC, "8080", []*corev1.ServiceAccount{saA, saB}, nil, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, podB, saC, svcC, podC1, podC2, epC)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specfake.NewSimpleClientset()

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-multi-sources-destinations.json", got)
}

func TestTopologyBuilder_EmptyTrafficTargetDestinationNamespace(t *testing.T) {
	namespace := "foo"
	tt := &access.TrafficTarget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficTarget",
			APIVersion: "access.smi-spec.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test",
		},
		Destination: access.IdentityBindingSubject{
			Kind: "ServiceAccount",
			Name: "test",
			Port: "80",
		},
	}

	k8sClient := fake.NewSimpleClientset()
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specfake.NewSimpleClientset()

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	res, err := builder.loadResources(resourceFilter)
	require.NoError(t, err)

	actual, exists := res.TrafficTargets[Key{Name: "test", Namespace: namespace}]
	assert.Equal(t, true, exists)
	assert.Equal(t, namespace, actual.Destination.Namespace)
}

// createBuilder initializes the different k8s factories and start them, initializes listers and create
// a new topology.Builder.
func createBuilder(k8sClient k8s.Interface, smiAccessClient accessclient.Interface, smiSpecClient specsclient.Interface, smiSplitClient splitclient.Interface) (*Builder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	k8sFactory := informers.NewSharedInformerFactoryWithOptions(k8sClient, mk8s.ResyncPeriod)

	svcLister := k8sFactory.Core().V1().Services().Lister()
	podLister := k8sFactory.Core().V1().Pods().Lister()
	epLister := k8sFactory.Core().V1().Endpoints().Lister()

	accessFactory := accessinformer.NewSharedInformerFactoryWithOptions(smiAccessClient, mk8s.ResyncPeriod)
	splitFactory := splitinformer.NewSharedInformerFactoryWithOptions(smiSplitClient, mk8s.ResyncPeriod)
	specsFactory := specsinformer.NewSharedInformerFactoryWithOptions(smiSpecClient, mk8s.ResyncPeriod)

	trafficTargetLister := accessFactory.Access().V1alpha1().TrafficTargets().Lister()
	trafficSplitLister := splitFactory.Split().V1alpha2().TrafficSplits().Lister()
	httpRouteGroupLister := specsFactory.Specs().V1alpha1().HTTPRouteGroups().Lister()
	tcpRouteLister := specsFactory.Specs().V1alpha1().TCPRoutes().Lister()

	k8sFactory.Start(ctx.Done())
	accessFactory.Start(ctx.Done())
	splitFactory.Start(ctx.Done())
	specsFactory.Start(ctx.Done())

	for t, ok := range k8sFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out while waiting for cache sync: %s", t.String())
		}
	}

	for t, ok := range accessFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out while waiting for cache sync: %s", t.String())
		}
	}

	for t, ok := range splitFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out while waiting for cache sync: %s", t.String())
		}
	}

	for t, ok := range specsFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out while waiting for cache sync: %s", t.String())
		}
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	return &Builder{
		serviceLister:        svcLister,
		endpointsLister:      epLister,
		podLister:            podLister,
		trafficTargetLister:  trafficTargetLister,
		trafficSplitLister:   trafficSplitLister,
		httpRouteGroupLister: httpRouteGroupLister,
		tcpRoutesLister:      tcpRouteLister,
		logger:               logger,
	}, nil
}

func nn(name, ns string) Key {
	return Key{
		Name:      name,
		Namespace: ns,
	}
}

func svcPort(name string, port, targetPort int32) corev1.ServicePort {
	return corev1.ServicePort{
		Name:       name,
		Protocol:   "TCP",
		Port:       port,
		TargetPort: intstr.FromInt(int(targetPort)),
	}
}

func createTrafficSplit(namespace, name string, svc *corev1.Service, backend1 *corev1.Service, backend2 *corev1.Service) *split.TrafficSplit {
	return &split.TrafficSplit{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficSplit",
			APIVersion: "split.smi-spec.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: split.TrafficSplitSpec{
			Service: svc.Name,
			Backends: []split.TrafficSplitBackend{
				{
					Service: backend1.Name,
					Weight:  80,
				},
				{
					Service: backend2.Name,
					Weight:  20,
				},
			},
		},
	}
}

func createTrafficTarget(namespace, name string, destSa *corev1.ServiceAccount, destPort string, srcsSa []*corev1.ServiceAccount, rtGrp *spec.HTTPRouteGroup, rtGrpMatches []string) *access.TrafficTarget {
	sources := make([]access.IdentityBindingSubject, len(srcsSa))
	for i, sa := range srcsSa {
		sources[i] = access.IdentityBindingSubject{
			Kind:      "ServiceAccount",
			Name:      sa.Name,
			Namespace: sa.Namespace,
		}
	}

	var specs []access.TrafficTargetSpec

	if rtGrp != nil {
		specs = append(specs, access.TrafficTargetSpec{
			Kind:    "HTTPRouteGroup",
			Name:    rtGrp.Name,
			Matches: rtGrpMatches,
		})
	}

	return &access.TrafficTarget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficTarget",
			APIVersion: "access.smi-spec.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Destination: access.IdentityBindingSubject{
			Kind:      "ServiceAccount",
			Name:      destSa.Name,
			Namespace: destSa.Namespace,
			Port:      destPort,
		},
		Sources: sources,
		Specs:   specs,
	}
}

func createHTTPRouteGroup(namespace, name string, matches []spec.HTTPMatch) *spec.HTTPRouteGroup {
	return &spec.HTTPRouteGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRouteGroup",
			APIVersion: "specs.smi-spec.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Matches: matches,
	}
}

func createHTTPMatch(name string, methods []string, pathPrefix string) spec.HTTPMatch {
	return spec.HTTPMatch{
		Name:      name,
		Methods:   methods,
		PathRegex: pathPrefix,
	}
}

func createService(namespace, name string, annotations map[string]string, targetPorts []corev1.ServicePort, selector map[string]string, clusterIP string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports:     targetPorts,
			Selector:  selector,
			ClusterIP: clusterIP,
			Type:      "ClusterIP",
		},
	}
}

func createEndpoints(svc *corev1.Service, pods []*corev1.Pod) *corev1.Endpoints {
	ports := make([]corev1.EndpointPort, len(svc.Spec.Ports))
	for i, port := range svc.Spec.Ports {
		ports[i] = corev1.EndpointPort{
			Name:     port.Name,
			Port:     port.TargetPort.IntVal,
			Protocol: port.Protocol,
		}
	}

	addresses := make([]corev1.EndpointAddress, len(pods))
	for i, pod := range pods {
		addresses[i] = corev1.EndpointAddress{
			IP: pod.Status.PodIP,
			TargetRef: &corev1.ObjectReference{
				Kind:            pod.Kind,
				Namespace:       pod.Namespace,
				Name:            pod.Name,
				APIVersion:      pod.APIVersion,
				ResourceVersion: pod.ResourceVersion,
			},
		}
	}

	return &corev1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: addresses,
				Ports:     ports,
			},
		},
	}
}

func createPod(namespace, name string, sa *corev1.ServiceAccount, selector map[string]string, podIP string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    selector,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: sa.Name,
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
		},
	}
}

func createServiceAccount(namespace, name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func assertTopology(t *testing.T, filename string, got *Topology) {
	data, err := ioutil.ReadFile(filename)
	require.NoError(t, err)

	var want Topology

	err = json.Unmarshal(data, &want)
	require.NoError(t, err)

	wantMarshaled, err := json.MarshalIndent(&want, "", "  ")
	require.NoError(t, err)

	gotMarshaled, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)

	assert.Equal(t, string(wantMarshaled), string(gotMarshaled))
}
