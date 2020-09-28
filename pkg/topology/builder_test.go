package topology

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
	"testing"
	"time"

	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha3"
	accessclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	accessfake "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned/fake"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	specsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	specsfake "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/fake"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	splitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	splitfake "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned/fake"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mk8s "github.com/traefik/mesh/v2/pkg/k8s"
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
		"mesh.traefik.io/traffic-type":      "http",
		"mesh.traefik.io/ratelimit-average": "100",
		"mesh.traefik.io/ratelimit-burst":   "200",
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

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", nil)
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric", nil)
	rtGrp := createHTTPRouteGroup("ignored-ns", "http-rt-grp-ignored", []specs.HTTPMatch{apiMatch, metricMatch})

	tt := createTrafficTarget("ignored-ns", "tt", saB, intPtr(8080), []*corev1.ServiceAccount{saA}, rtGrp, []string{})
	ts := createTrafficSplit("ignored-ns", "ts", svcB, svcC, svcD, nil)

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, svcB, podB, svcC, svcD)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset(ts)
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

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

	epB := createEndpoints(svcB, createEndpointSubset(svcPorts, podB))
	epC := createEndpoints(svcC, createEndpointSubset(svcPorts, podC))
	epD := createEndpoints(svcD, createEndpointSubset(svcPorts, podD))
	epE := createEndpoints(svcE, createEndpointSubset(svcPorts, podE))

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", nil)
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric", nil)
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []specs.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	ttb := createTrafficTarget("my-ns", "tt-b", saB, intPtr(8080), []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttd := createTrafficTarget("my-ns", "tt-d", saD, intPtr(8080), []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, intPtr(8080), []*corev1.ServiceAccount{saA1, saA2}, rtGrp, ttMatch)
	tte := createTrafficTarget("my-ns", "tt-e", saE, intPtr(8080), []*corev1.ServiceAccount{saA2}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD, nil)
	tsErr := createTrafficSplit("my-ns", "tsErr", svcC, svcB, svcE, nil)

	k8sClient := fake.NewSimpleClientset(saA1, saA2, saB, saC, saD, saE,
		podA1, podA2, podB, podC, podD, podE,
		svcB, svcC, svcD, svcE,
		epB, epC, epD, epE)
	smiAccessClient := accessfake.NewSimpleClientset(ttb, ttc, ttd, tte)
	smiSplitClient := splitfake.NewSimpleClientset(ts, tsErr)
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

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
		"mesh.traefik.io/traffic-type":      "http",
		"mesh.traefik.io/ratelimit-average": "100",
		"mesh.traefik.io/ratelimit-burst":   "200",
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

	epB := createEndpoints(svcB, createEndpointSubset(svcPorts, podB))
	epC := createEndpoints(svcC, createEndpointSubset(svcPorts, podC))
	epD := createEndpoints(svcD, createEndpointSubset(svcPorts, podD))

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", nil)
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric", nil)
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []specs.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	tt := createTrafficTarget("my-ns", "tt", saB, intPtr(8080), []*corev1.ServiceAccount{saA}, rtGrp, ttMatch)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, intPtr(8080), []*corev1.ServiceAccount{saA, saA2}, rtGrp, ttMatch)
	ttd := createTrafficTarget("my-ns", "tt-d", saD, intPtr(8080), []*corev1.ServiceAccount{saC}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD, nil)

	k8sClient := fake.NewSimpleClientset(saA, podA, podA2, saB, svcB, podB, svcC, svcD, podC, podD, epB, epC, epD)
	smiAccessClient := accessfake.NewSimpleClientset(tt, ttc, ttd)
	smiSplitClient := splitfake.NewSimpleClientset(ts)
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

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

	epB := createEndpoints(svcB, createEndpointSubset(svcPorts, podB))
	epC := createEndpoints(svcC, createEndpointSubset(svcPorts, podC))
	epD := createEndpoints(svcD, createEndpointSubset(svcPorts, podD))
	epE := createEndpoints(svcE, createEndpointSubset(svcPorts, podE))

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", nil)
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric", nil)
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []specs.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	ttb := createTrafficTarget("my-ns", "tt-b", saB, intPtr(8080), []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttd := createTrafficTarget("my-ns", "tt-d", saD, intPtr(8080), []*corev1.ServiceAccount{saA1}, rtGrp, ttMatch)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, intPtr(8080), []*corev1.ServiceAccount{saA1, saA2}, rtGrp, ttMatch)
	tte := createTrafficTarget("my-ns", "tt-e", saE, intPtr(8080), []*corev1.ServiceAccount{saA2}, rtGrp, ttMatch)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD, nil)
	ts2 := createTrafficSplit("my-ns", "ts2", svcB, svcC, svcE, nil)

	k8sClient := fake.NewSimpleClientset(saA1, saA2, saB, saC, saD, saE,
		podA1, podA2, podB, podC, podD, podE,
		svcB, svcC, svcD, svcE,
		epB, epC, epD, epE)
	smiAccessClient := accessfake.NewSimpleClientset(ttb, ttc, ttd, tte)
	smiSplitClient := splitfake.NewSimpleClientset(ts, ts2)
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	svcKey := nn(svcB.Name, svcB.Namespace)
	tsKeys := got.Services[svcKey].TrafficSplits

	assert.Len(t, tsKeys, 2)
	assert.Len(t, got.TrafficSplits, 2)

	assertTopology(t, "testdata/topology-traffic-split-traffic-target.json", got)
}

// TestTopologyBuilder_EvaluatesTrafficSplitSpecs makes sure a topology can be built with TrafficSplits containing
// HTTPRouteGroups.
func TestTopologyBuilder_EvaluatesTrafficSplitSpecs(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	selectorAppC := map[string]string{"app": "app-c"}
	selectorAppD := map[string]string{"app": "app-d"}
	annotations := map[string]string{}
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

	epB := createEndpoints(svcB, createEndpointSubset(svcPorts, podB))
	epC := createEndpoints(svcC, createEndpointSubset(svcPorts, podC))
	epD := createEndpoints(svcD, createEndpointSubset(svcPorts, podD))

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", nil)
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []specs.HTTPMatch{apiMatch})

	ttd := createTrafficTarget("my-ns", "tt-d", saD, intPtr(8080), []*corev1.ServiceAccount{saA}, nil, nil)
	ttc := createTrafficTarget("my-ns", "tt-c", saC, intPtr(8080), []*corev1.ServiceAccount{saA}, nil, nil)
	ts := createTrafficSplit("my-ns", "ts", svcB, svcC, svcD, rtGrp)

	k8sClient := fake.NewSimpleClientset(saA, saB, saC, saD,
		podA, podB, podC, podD,
		svcB, svcC, svcD,
		epB, epC, epD)
	smiAccessClient := accessfake.NewSimpleClientset(ttc, ttd)
	smiSplitClient := splitfake.NewSimpleClientset(ts)
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-traffic-split-specs.json", got)
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

	epB := createEndpoints(svcB, createEndpointSubset(svcPorts, podB))

	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric", nil)
	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", map[string]string{
		"User-Agent": "curl/.*",
	})
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []specs.HTTPMatch{apiMatch, metricMatch})

	ttMatch := []string{apiMatch.Name}
	tt := createTrafficTarget("my-ns", "tt", saB, intPtr(8080), []*corev1.ServiceAccount{saA}, rtGrp, ttMatch)

	k8sClient := fake.NewSimpleClientset(saA, saB, podA, podB, svcB, epB)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-traffic-target.json", got)
}

// TestTopologyBuilder_BuildWithTrafficTargetSpecEmptyMatch makes sure that when TrafficTarget.Spec.Matches is empty,
// the output list contains all the matches defined in the HTTPRouteGroup (as defined by the
// spec https://github.com/servicemeshinterface/smi-spec/tree/master/apis/traffic-access/v1alpha2)
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

	epB := createEndpoints(svcB, createEndpointSubset(svcbPorts, podB))

	apiMatch := createHTTPMatch("api", []string{"GET", "POST"}, "/api", nil)
	metricMatch := createHTTPMatch("metric", []string{"GET"}, "/metric", nil)
	rtGrp := createHTTPRouteGroup("my-ns", "http-rt-grp", []specs.HTTPMatch{apiMatch, metricMatch})

	tt := createTrafficTarget("my-ns", "tt", saB, intPtr(8080), []*corev1.ServiceAccount{saA}, rtGrp, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, svcB, podB, epB)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset(rtGrp)

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-spec-with-empty-match.json", got)
}

// TestTopologyBuilder_BuildWithTrafficTargetEmptyDestinationPort makes sure that when a TrafficTarget.Destination.Port
// is empty, the output contains all the ports defined by the destination service (as defined by the
// spec https://github.com/servicemeshinterface/smi-spec/tree/master/apis/traffic-access/v1alpha2)
func TestTopologyBuilder_BuildWithTrafficTargetEmptyDestinationPort(t *testing.T) {
	selectorAppA := map[string]string{"app": "app-a"}
	selectorAppB := map[string]string{"app": "app-b"}
	annotations := map[string]string{
		"mesh.traefik.io/traffic-type":      "http",
		"mesh.traefik.io/ratelimit-average": "100",
		"mesh.traefik.io/ratelimit-burst":   "200",
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

	epB := createEndpoints(svcB, createEndpointSubset(svcbPorts, podB))
	tt := createTrafficTarget("my-ns", "tt", saB, nil, []*corev1.ServiceAccount{saA}, nil, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, svcB, podB, epB)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset()

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
	epB1 := createEndpoints(svcB1, createEndpointSubset(svcB1Ports, podB1))

	selectorAppB2 := map[string]string{"app": "app-b2"}
	svcB2Ports := []corev1.ServicePort{svcPort("port-80", 80, 80)}
	podB2 := createPod("my-ns", "app-b2", saB, selectorAppB2, "10.10.1.3")
	svcB2 := createService("my-ns", "svc-b2", annotations, svcB2Ports, selectorAppB2, "10.10.1.17")
	epB2 := createEndpoints(svcB2, createEndpointSubset(svcB2Ports, podB2))

	tt := createTrafficTarget("my-ns", "tt", saB, intPtr(80), []*corev1.ServiceAccount{saA}, nil, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, podB1, svcB1, epB1, podB2, svcB2, epB2)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset()

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

	epC := createEndpoints(svcC, createEndpointSubset(svccPorts, podC1, podC2))

	tt := createTrafficTarget("my-ns", "tt", saC, intPtr(8080), []*corev1.ServiceAccount{saA, saB}, nil, []string{})

	k8sClient := fake.NewSimpleClientset(saA, podA, saB, podB, saC, svcC, podC1, podC2, epC)
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset()

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
			APIVersion: "access.smi-spec.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test",
		},
		Spec: access.TrafficTargetSpec{
			Destination: access.IdentityBindingSubject{
				Kind: "ServiceAccount",
				Name: "test",
				Port: intPtr(80),
			},
		},
	}

	k8sClient := fake.NewSimpleClientset()
	smiAccessClient := accessfake.NewSimpleClientset(tt)
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset()

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()
	res, err := builder.loadResources(resourceFilter)
	require.NoError(t, err)

	actual, exists := res.TrafficTargets[Key{Name: "test", Namespace: namespace}]
	assert.Equal(t, true, exists)
	assert.Equal(t, namespace, actual.Spec.Destination.Namespace)
}

func TestTopologyBuilder_BuildServiceWithPodPortMixture(t *testing.T) {
	serviceAccount := createServiceAccount("my-ns", "service-account")

	podV1 := createPod(
		"my-ns",
		"pod-v1",
		serviceAccount,
		map[string]string{"app": "my-app", "version": "v1"},
		"10.10.1.1",
	)

	podV2 := createPod(
		"my-ns",
		"pod-v2",
		serviceAccount,
		map[string]string{"app": "my-app", "version": "v2"},
		"10.10.1.2",
	)

	svcPorts := []corev1.ServicePort{
		{
			Name:       "port-80",
			Port:       80,
			TargetPort: intstr.FromString("name"),
		},
		{
			Name:       "port-8080",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		},
	}

	svc := createService(
		"my-ns",
		"svc",
		nil,
		svcPorts,
		map[string]string{"app": "my-app"},
		"10.10.1.3",
	)

	endpoints := createEndpoints(svc,
		createEndpointSubset([]corev1.ServicePort{svcPorts[0]}, podV1),
		createEndpointSubset([]corev1.ServicePort{svcPorts[1]}, podV1, podV2),
	)

	k8sClient := fake.NewSimpleClientset(endpoints, svc, podV1, podV2)
	smiAccessClient := accessfake.NewSimpleClientset()
	smiSplitClient := splitfake.NewSimpleClientset()
	smiSpecClient := specsfake.NewSimpleClientset()

	builder, err := createBuilder(k8sClient, smiAccessClient, smiSpecClient, smiSplitClient)
	require.NoError(t, err)

	resourceFilter := mk8s.NewResourceFilter()

	got, err := builder.Build(resourceFilter)
	require.NoError(t, err)

	assertTopology(t, "testdata/topology-service-with-pod-port-mixture.json", got)
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

	trafficTargetLister := accessFactory.Access().V1alpha2().TrafficTargets().Lister()
	trafficSplitLister := splitFactory.Split().V1alpha3().TrafficSplits().Lister()
	httpRouteGroupLister := specsFactory.Specs().V1alpha3().HTTPRouteGroups().Lister()
	tcpRouteLister := specsFactory.Specs().V1alpha3().TCPRoutes().Lister()

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

func createTrafficSplit(namespace, name string, svc *corev1.Service, backend1 *corev1.Service, backend2 *corev1.Service, rtGrp *specs.HTTPRouteGroup) *split.TrafficSplit {
	var matches []corev1.TypedLocalObjectReference

	if rtGrp != nil {
		matches = append(matches, corev1.TypedLocalObjectReference{
			Kind: rtGrp.Kind,
			Name: rtGrp.Name,
		})
	}

	return &split.TrafficSplit{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficSplit",
			APIVersion: "split.smi-spec.io/v1alpha3",
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
			Matches: matches,
		},
	}
}

func createTrafficTarget(namespace, name string, destSa *corev1.ServiceAccount, destPort *int, srcsSa []*corev1.ServiceAccount, rtGrp *specs.HTTPRouteGroup, rtGrpMatches []string) *access.TrafficTarget {
	sources := make([]access.IdentityBindingSubject, len(srcsSa))
	for i, sa := range srcsSa {
		sources[i] = access.IdentityBindingSubject{
			Kind:      "ServiceAccount",
			Name:      sa.Name,
			Namespace: sa.Namespace,
		}
	}

	var rules []access.TrafficTargetRule

	if rtGrp != nil {
		rules = append(rules, access.TrafficTargetRule{
			Kind:    "HTTPRouteGroup",
			Name:    rtGrp.Name,
			Matches: rtGrpMatches,
		})
	}

	return &access.TrafficTarget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficTarget",
			APIVersion: "access.smi-spec.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: access.TrafficTargetSpec{
			Destination: access.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      destSa.Name,
				Namespace: destSa.Namespace,
				Port:      destPort,
			},
			Sources: sources,
			Rules:   rules,
		},
	}
}

func createHTTPRouteGroup(namespace, name string, matches []specs.HTTPMatch) *specs.HTTPRouteGroup {
	return &specs.HTTPRouteGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRouteGroup",
			APIVersion: "specs.smi-spec.io/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: specs.HTTPRouteGroupSpec{
			Matches: matches,
		},
	}
}

func createHTTPMatch(name string, methods []string, pathPrefix string, headers map[string]string) specs.HTTPMatch {
	return specs.HTTPMatch{
		Name:      name,
		Methods:   methods,
		PathRegex: pathPrefix,
		Headers:   headers,
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

func createEndpointSubset(svcPorts []corev1.ServicePort, pods ...*corev1.Pod) corev1.EndpointSubset {
	ports := make([]corev1.EndpointPort, len(svcPorts))
	for i, port := range svcPorts {
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

	return corev1.EndpointSubset{
		Addresses: addresses,
		Ports:     ports,
	}
}

func createEndpoints(svc *corev1.Service, subsets ...corev1.EndpointSubset) *corev1.Endpoints {
	return &corev1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
		Subsets: subsets,
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

	// Sort slices which order may be affected by a map iteration.
	for _, svc := range got.Services {
		sort.Slice(svc.TrafficSplits, buildKeySorter(svc.TrafficSplits))
		sort.Slice(svc.Pods, buildKeySorter(svc.Pods))
		sort.Slice(svc.BackendOf, buildKeySorter(svc.BackendOf))
		sort.Slice(svc.TrafficTargets, buildServiceTrafficTargetKeySorter(svc.TrafficTargets))
	}

	for _, pod := range got.Pods {
		sort.Slice(pod.SourceOf, buildServiceTrafficTargetKeySorter(pod.SourceOf))
		sort.Slice(pod.DestinationOf, buildServiceTrafficTargetKeySorter(pod.DestinationOf))
	}

	gotMarshaled, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)

	assert.Equal(t, string(wantMarshaled), string(gotMarshaled))
}

func buildKeySorter(keys []Key) func(i, j int) bool {
	return func(i, j int) bool {
		return strings.Compare(keys[i].String(), keys[j].String()) < 0
	}
}

func buildServiceTrafficTargetKeySorter(keys []ServiceTrafficTargetKey) func(i, j int) bool {
	return func(i, j int) bool {
		return strings.Compare(keys[i].String(), keys[j].String()) < 0
	}
}

func intPtr(value int) *int {
	return &value
}
