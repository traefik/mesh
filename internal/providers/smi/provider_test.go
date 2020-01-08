package smi

import (
	"context"
	"testing"

	"github.com/containous/maesh/internal/providers/base"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestBuildRuleSnippetFromServiceAndMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
	ignored := k8s.NewIgnored()
	provider := New(k8s.ServiceTypeHTTP, nil, ignored,
		clientMock.ServiceLister,
		clientMock.EndpointsLister,
		clientMock.PodLister,
		clientMock.TrafficTargetLister,
		clientMock.HTTPRouteGroupLister,
		clientMock.TCPRouteLister,
		clientMock.TrafficSplitLister)

	testCases := []struct {
		desc     string
		expected string
		match    specsv1alpha1.HTTPMatch
	}{
		{
			desc:     "method and regex in match",
			expected: "PathPrefix(`/foo`) && Method(`GET`,`POST`) && (Host(`test.foo.maesh`) || Host(`10.0.0.1`))",
			match: specsv1alpha1.HTTPMatch{
				Name:      "test",
				Methods:   []string{"GET", "POST"},
				PathRegex: "/foo",
			},
		},
		{
			desc:     "method only in match",
			expected: "Method(`GET`,`POST`) && (Host(`test.foo.maesh`) || Host(`10.0.0.1`))",
			match: specsv1alpha1.HTTPMatch{
				Name:    "test",
				Methods: []string{"GET", "POST"},
			},
		},
		{
			desc:     "prefix only in match",
			expected: "PathPrefix(`/foo`) && (Host(`test.foo.maesh`) || Host(`10.0.0.1`))",
			match: specsv1alpha1.HTTPMatch{
				Name:      "test",
				PathRegex: "/foo",
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			name := "test"
			namespace := "foo"
			ip := "10.0.0.1"
			actual := provider.buildRuleSnippetFromServiceAndMatch(name, namespace, ip, test.match)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetTrafficTargetsWithDestinationInNamespace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
	ignored := k8s.NewIgnored()
	provider := New(k8s.ServiceTypeHTTP, nil, ignored,
		clientMock.ServiceLister,
		clientMock.EndpointsLister,
		clientMock.PodLister,
		clientMock.TrafficTargetLister,
		clientMock.HTTPRouteGroupLister,
		clientMock.TCPRouteLister,
		clientMock.TrafficSplitLister)

	expected := []*accessv1alpha1.TrafficTarget{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-service-metrics-2",
				Namespace: metav1.NamespaceDefault,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "TrafficTarget",
				APIVersion: "access.smi-spec.io/v1alpha1",
			},
			Destination: accessv1alpha1.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      "api-service",
				Namespace: "foo",
			},
			Sources: []accessv1alpha1.IdentityBindingSubject{
				{
					Kind:      "ServiceAccount",
					Name:      "prometheus",
					Namespace: metav1.NamespaceDefault,
				},
			},
			Specs: []accessv1alpha1.TrafficTargetSpec{
				{
					Kind:    "HTTPRouteGroup",
					Name:    "api-service-routes",
					Matches: []string{"metrics"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-service-api-2",
				Namespace: metav1.NamespaceDefault,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "TrafficTarget",
				APIVersion: "access.smi-spec.io/v1alpha1",
			},
			Destination: accessv1alpha1.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      "api-service",
				Namespace: "foo",
				Port:      "8080",
			},
			Sources: []accessv1alpha1.IdentityBindingSubject{
				{
					Kind:      "ServiceAccount",
					Name:      "website-service",
					Namespace: metav1.NamespaceDefault,
				},
				{
					Kind:      "ServiceAccount",
					Name:      "payments-service",
					Namespace: metav1.NamespaceDefault,
				},
			},
			Specs: []accessv1alpha1.TrafficTargetSpec{
				{
					Kind:    "HTTPRouteGroup",
					Name:    "api-service-routes",
					Matches: []string{"api"},
				},
			},
		},
	}
	allTrafficTargets, err := clientMock.TrafficTargetLister.TrafficTargets(metav1.NamespaceAll).List(labels.Everything())
	assert.NoError(t, err)

	actual := provider.getTrafficTargetsWithDestinationInNamespace("foo", allTrafficTargets)
	assert.Equal(t, len(actual), len(expected))

	for _, expectedValue := range expected {
		assert.Contains(t, actual, expectedValue)
	}
}

func TestBuildHTTPRouterFromTrafficTarget(t *testing.T) {
	testCases := []struct {
		desc             string
		serviceName      string
		serviceNamespace string
		serviceIP        string
		port             int
		key              string
		trafficTarget    *accessv1alpha1.TrafficTarget
		expected         *dynamic.Router
		httpError        bool
	}{
		{
			desc:             "simple router",
			serviceName:      "test",
			serviceNamespace: metav1.NamespaceDefault,
			serviceIP:        "10.0.0.1",
			port:             81,
			key:              "example",
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-metrics-2",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.Router{
				EntryPoints: []string{"http-81"},
				Service:     "example",
				Rule:        "(PathPrefix(`/metrics`) && Method(`GET`) && (Host(`test.default.maesh`) || Host(`10.0.0.1`)))",
				Middlewares: []string{"block-all"},
			},
		},
		{
			desc:             "simple router missing HTTPRouteGroup",
			serviceName:      "test",
			serviceNamespace: metav1.NamespaceDefault,
			serviceIP:        "10.0.0.1",
			port:             81,
			key:              "example",
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-metrics-2",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-foo",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.Router{
				EntryPoints: []string{"http-81"},
				Service:     "example",
				Middlewares: []string{"block-all"},
			},
		},
		{
			desc:             "simple router unsupported spec kind",
			serviceName:      "test",
			serviceNamespace: metav1.NamespaceDefault,
			serviceIP:        "10.0.0.1",
			port:             81,
			key:              "example",
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-metrics-2",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "Bacon",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.Router{
				EntryPoints: []string{"http-81"},
				Service:     "example",
				Middlewares: []string{"block-all"},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored,
				clientMock.ServiceLister,
				clientMock.EndpointsLister,
				clientMock.PodLister,
				clientMock.TrafficTargetLister,
				clientMock.HTTPRouteGroupLister,
				clientMock.TCPRouteLister,
				clientMock.TrafficSplitLister)
			middleware := "block-all"
			actual := provider.buildHTTPRouterFromTrafficTarget(test.serviceName, test.serviceNamespace, test.serviceIP, test.trafficTarget, test.port, test.key, middleware)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildTCPRouterFromTrafficTarget(t *testing.T) {
	testCases := []struct {
		desc          string
		port          int
		key           string
		trafficTarget *accessv1alpha1.TrafficTarget
		expected      *dynamic.TCPRouter
		tcpError      bool
	}{
		{
			desc: "simple router",
			port: 80,
			key:  "example",
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-traffic-target",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind: "TCPRoute",
						Name: "api-service-routes",
					},
				},
			},
			expected: &dynamic.TCPRouter{
				EntryPoints: []string{"tcp-80"},
				Service:     "example",
				Rule:        "HostSNI(`*`)",
			},
		},
		{
			desc: "simple router missing TCPRoute",
			port: 81,
			key:  "example",
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-metrics-2",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind: "TCPService",
						Name: "api-service-foo",
					},
				},
			},
			expected: &dynamic.TCPRouter{
				EntryPoints: []string{"tcp-81"},
				Service:     "example",
			},
		},
		{
			desc: "simple router with TCPRoute error",
			port: 81,
			key:  "example",
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-metrics-2",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.TCPRouter{
				EntryPoints: []string{"tcp-81"},
				Service:     "example",
			},
			tcpError: true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), "mock_tcp.yaml", true)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored,
				clientMock.ServiceLister,
				clientMock.EndpointsLister,
				clientMock.PodLister,
				clientMock.TrafficTargetLister,
				clientMock.HTTPRouteGroupLister,
				clientMock.TCPRouteLister,
				clientMock.TrafficSplitLister)
			actual := provider.buildTCPRouterFromTrafficTarget(test.trafficTarget, test.port, test.key)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetServiceMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
	ignored := k8s.NewIgnored()
	provider := New(k8s.ServiceTypeHTTP, nil, ignored,
		clientMock.ServiceLister,
		clientMock.EndpointsLister,
		clientMock.PodLister,
		clientMock.TrafficTargetLister,
		clientMock.HTTPRouteGroupLister,
		clientMock.TCPRouteLister,
		clientMock.TrafficSplitLister)

	testCases := []struct {
		desc     string
		expected string
		provided string
	}{
		{
			desc:     "empty provided",
			expected: k8s.ServiceTypeHTTP,
			provided: "",
		},
		{
			desc:     "same provided",
			expected: k8s.ServiceTypeHTTP,
			provided: k8s.ServiceTypeHTTP,
		},
		{
			desc:     "different provided",
			expected: k8s.ServiceTypeTCP,
			provided: k8s.ServiceTypeTCP,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			actual := provider.getServiceMode(test.provided)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetApplicableTrafficTargets(t *testing.T) {
	testCases := []struct {
		desc           string
		endpoints      *corev1.Endpoints
		trafficTargets []*accessv1alpha1.TrafficTarget
		expected       []*accessv1alpha1.TrafficTarget
	}{
		{
			desc: "traffictarget destination in different namespace",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-service",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.50",
								TargetRef: &corev1.ObjectReference{
									Name:      "example",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 50,
							},
						},
					},
				},
			},
			trafficTargets: []*accessv1alpha1.TrafficTarget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-foo",
						Namespace: metav1.NamespaceDefault,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "TrafficTarget",
						APIVersion: "access.smi-spec.io/v1alpha1",
					},
					Destination: accessv1alpha1.IdentityBindingSubject{
						Kind:      "ServiceAccount",
						Name:      "api-service",
						Namespace: "foo",
					},
					Sources: []accessv1alpha1.IdentityBindingSubject{
						{
							Kind:      "ServiceAccount",
							Name:      "prometheus",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Specs: []accessv1alpha1.TrafficTargetSpec{
						{
							Kind:    "HTTPRouteGroup",
							Name:    "api-service-routes",
							Matches: []string{"metrics"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			desc: "valid traffictarget found",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-service",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.50",
								TargetRef: &corev1.ObjectReference{
									Name:      "example",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 50,
							},
						},
					},
				},
			},
			trafficTargets: []*accessv1alpha1.TrafficTarget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-foo",
						Namespace: metav1.NamespaceDefault,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "TrafficTarget",
						APIVersion: "access.smi-spec.io/v1alpha1",
					},
					Destination: accessv1alpha1.IdentityBindingSubject{
						Kind:      "ServiceAccount",
						Name:      "api-service",
						Namespace: metav1.NamespaceDefault,
					},
					Sources: []accessv1alpha1.IdentityBindingSubject{
						{
							Kind:      "ServiceAccount",
							Name:      "prometheus",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Specs: []accessv1alpha1.TrafficTargetSpec{
						{
							Kind:    "HTTPRouteGroup",
							Name:    "api-service-routes",
							Matches: []string{"metrics"},
						},
					},
				},
			},
			expected: []*accessv1alpha1.TrafficTarget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-foo",
						Namespace: metav1.NamespaceDefault,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "TrafficTarget",
						APIVersion: "access.smi-spec.io/v1alpha1",
					},
					Destination: accessv1alpha1.IdentityBindingSubject{
						Kind:      "ServiceAccount",
						Name:      "api-service",
						Namespace: metav1.NamespaceDefault,
					},
					Sources: []accessv1alpha1.IdentityBindingSubject{
						{
							Kind:      "ServiceAccount",
							Name:      "prometheus",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Specs: []accessv1alpha1.TrafficTargetSpec{
						{
							Kind:    "HTTPRouteGroup",
							Name:    "api-service-routes",
							Matches: []string{"metrics"},
						},
					},
				},
			},
		},
		{
			desc: "no subset match",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-service",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.50",
								TargetRef: &corev1.ObjectReference{
									Name:      "example",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 50,
							},
						},
					},
				},
			},
			trafficTargets: []*accessv1alpha1.TrafficTarget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-foo",
						Namespace: metav1.NamespaceDefault,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "TrafficTarget",
						APIVersion: "access.smi-spec.io/v1alpha1",
					},
					Destination: accessv1alpha1.IdentityBindingSubject{
						Kind:      "ServiceAccount",
						Name:      "api-service",
						Namespace: metav1.NamespaceDefault,
						Port:      "5000",
					},
					Sources: []accessv1alpha1.IdentityBindingSubject{
						{
							Kind:      "ServiceAccount",
							Name:      "prometheus",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Specs: []accessv1alpha1.TrafficTargetSpec{
						{
							Kind:    "HTTPRouteGroup",
							Name:    "api-service-routes",
							Matches: []string{"metrics"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			desc: "pod doesnt exist error",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-service",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.50",
								TargetRef: &corev1.ObjectReference{
									Name:      "foo",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 50,
							},
						},
					},
				},
			},
			trafficTargets: []*accessv1alpha1.TrafficTarget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-foo",
						Namespace: metav1.NamespaceDefault,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "TrafficTarget",
						APIVersion: "access.smi-spec.io/v1alpha1",
					},
					Destination: accessv1alpha1.IdentityBindingSubject{
						Kind:      "ServiceAccount",
						Name:      "api-service",
						Namespace: metav1.NamespaceDefault,
					},
					Sources: []accessv1alpha1.IdentityBindingSubject{
						{
							Kind:      "ServiceAccount",
							Name:      "prometheus",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Specs: []accessv1alpha1.TrafficTargetSpec{
						{
							Kind:    "HTTPRouteGroup",
							Name:    "api-service-routes",
							Matches: []string{"metrics"},
						},
					},
				},
			},
			expected: nil,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored,
				clientMock.ServiceLister,
				clientMock.EndpointsLister,
				clientMock.PodLister,
				clientMock.TrafficTargetLister,
				clientMock.HTTPRouteGroupLister,
				clientMock.TCPRouteLister,
				clientMock.TrafficSplitLister)

			actual := provider.getApplicableTrafficTargets(test.endpoints, test.trafficTargets)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildHTTPServiceFromTrafficTarget(t *testing.T) {
	testCases := []struct {
		desc          string
		endpoints     *corev1.Endpoints
		trafficTarget *accessv1alpha1.TrafficTarget
		expected      *dynamic.Service
		podError      bool
	}{
		{
			desc: "successful service",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.10",
								TargetRef: &corev1.ObjectReference{
									Name:      "example",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 5080,
							},
						},
					},
				},
			},
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-foo",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: metav1.NamespaceDefault,
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.Service{
				LoadBalancer: &dynamic.ServersLoadBalancer{
					PassHostHeader: base.Bool(true),
					Servers: []dynamic.Server{
						{
							URL: "http://10.1.1.10:5080",
						},
					},
				},
			},
		},
		{
			desc: "mismatch namespace",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: metav1.NamespaceSystem,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.10",
								TargetRef: &corev1.ObjectReference{
									Name:      "example",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 5080,
							},
						},
					},
				},
			},
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-foo",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: metav1.NamespaceDefault,
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: nil,
		},
		{
			desc: "successful service",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.10",
								TargetRef: &corev1.ObjectReference{
									Name:      "example",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 5080,
							},
						},
					},
				},
			},
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-foo",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: metav1.NamespaceDefault,
					Port:      "10",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.Service{
				LoadBalancer: &dynamic.ServersLoadBalancer{
					PassHostHeader: base.Bool(true),
					Servers:        nil,
				},
			},
		},
		{
			desc: "pod does not exist",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: metav1.NamespaceDefault,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.1.1.10",
								TargetRef: &corev1.ObjectReference{
									Name:      "foo",
									Namespace: metav1.NamespaceDefault,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 5080,
							},
						},
					},
				},
			},
			trafficTarget: &accessv1alpha1.TrafficTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-foo",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: metav1.NamespaceDefault,
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			expected: &dynamic.Service{
				LoadBalancer: &dynamic.ServersLoadBalancer{
					PassHostHeader: base.Bool(true),
					Servers:        nil,
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored,
				clientMock.ServiceLister,
				clientMock.EndpointsLister,
				clientMock.PodLister,
				clientMock.TrafficTargetLister,
				clientMock.HTTPRouteGroupLister,
				clientMock.TCPRouteLister,
				clientMock.TrafficSplitLister)

			actual := provider.buildHTTPServiceFromTrafficTarget(test.endpoints, test.trafficTarget, k8s.SchemeHTTP)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGroupTrafficTargetsByDestination(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)
	ignored := k8s.NewIgnored()
	provider := New(k8s.ServiceTypeHTTP, nil, ignored,
		clientMock.ServiceLister,
		clientMock.EndpointsLister,
		clientMock.PodLister,
		clientMock.TrafficTargetLister,
		clientMock.HTTPRouteGroupLister,
		clientMock.TCPRouteLister,
		clientMock.TrafficSplitLister)

	trafficTargets := []*accessv1alpha1.TrafficTarget{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-service-metrics",
				Namespace: metav1.NamespaceDefault,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "TrafficTarget",
				APIVersion: "access.smi-spec.io/v1alpha1",
			},
			Destination: accessv1alpha1.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      "api-service",
				Namespace: "foo",
			},
			Sources: []accessv1alpha1.IdentityBindingSubject{
				{
					Kind:      "ServiceAccount",
					Name:      "prometheus",
					Namespace: metav1.NamespaceDefault,
				},
			},
			Specs: []accessv1alpha1.TrafficTargetSpec{
				{
					Kind:    "HTTPRouteGroup",
					Name:    "api-service-routes",
					Matches: []string{"metrics"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-service-api",
				Namespace: metav1.NamespaceDefault,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "TrafficTarget",
				APIVersion: "access.smi-spec.io/v1alpha1",
			},
			Destination: accessv1alpha1.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      "api-service",
				Namespace: "foo",
			},
			Sources: []accessv1alpha1.IdentityBindingSubject{
				{
					Kind:      "ServiceAccount",
					Name:      "website-service",
					Namespace: metav1.NamespaceDefault,
				},
				{
					Kind:      "ServiceAccount",
					Name:      "payments-service",
					Namespace: metav1.NamespaceDefault,
				},
			},
			Specs: []accessv1alpha1.TrafficTargetSpec{
				{
					Kind:    "HTTPRouteGroup",
					Name:    "api-service-routes",
					Matches: []string{"api"},
				},
			},
		},
	}

	expected := map[destinationKey][]*accessv1alpha1.TrafficTarget{
		{
			name:      "api-service",
			namespace: "foo",
			port:      "",
		}: {
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-metrics",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"metrics"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-service-api",
					Namespace: metav1.NamespaceDefault,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "TrafficTarget",
					APIVersion: "access.smi-spec.io/v1alpha1",
				},
				Destination: accessv1alpha1.IdentityBindingSubject{
					Kind:      "ServiceAccount",
					Name:      "api-service",
					Namespace: "foo",
				},
				Sources: []accessv1alpha1.IdentityBindingSubject{
					{
						Kind:      "ServiceAccount",
						Name:      "website-service",
						Namespace: metav1.NamespaceDefault,
					},
					{
						Kind:      "ServiceAccount",
						Name:      "payments-service",
						Namespace: metav1.NamespaceDefault,
					},
				},
				Specs: []accessv1alpha1.TrafficTargetSpec{
					{
						Kind:    "HTTPRouteGroup",
						Name:    "api-service-routes",
						Matches: []string{"api"},
					},
				},
			},
		},
	}
	actual := provider.groupTrafficTargetsByDestination(trafficTargets)
	assert.Equal(t, expected, actual)
}

func TestBuildConfiguration(t *testing.T) {
	testCases := []struct {
		desc           string
		mockFile       string
		expected       *dynamic.Configuration
		endpointsError bool
		serviceError   bool
	}{
		{
			desc:     "simple configuration build with HTTP service",
			mockFile: "build_configuration_http_service.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"demo-servi-default-80-api-servic-default-5bb66e727779b5ba": {
							EntryPoints: []string{"http-5000"},
							Rule:        "(PathPrefix(`/metrics`) && Method(`GET`) && (Host(`demo-service.default.maesh`) || Host(`10.1.0.1`)))",
							Service:     "demo-servi-default-80-api-servic-default-5bb66e727779b5ba",
							Middlewares: []string{"api-service-metrics-default-demo-servi-default-80-api-servic-default-5bb66e727779b5ba-whitelist"},
						},
						"readiness": {
							EntryPoints: []string{"readiness"},
							Service:     "readiness",
							Rule:        "Path(`/ping`)",
						},
					},
					Services: map[string]*dynamic.Service{
						"demo-servi-default-80-api-servic-default-5bb66e727779b5ba": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
								Servers: []dynamic.Server{
									{
										URL: "http://10.1.1.50:50",
									},
								},
							},
						},
						"readiness": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
								Servers: []dynamic.Server{
									{
										URL:    "http://127.0.0.1:8080",
										Scheme: "",
										Port:   "",
									},
								},
							},
						},
					},
					Middlewares: map[string]*dynamic.Middleware{
						"api-service-metrics-default-demo-servi-default-80-api-servic-default-5bb66e727779b5ba-whitelist": {
							IPWhiteList: &dynamic.IPWhiteList{
								SourceRange: []string{"10.4.3.100"},
							},
						},
						"smi-block-all-middleware": {
							IPWhiteList: &dynamic.IPWhiteList{
								SourceRange: []string{"255.255.255.255"},
							},
						},
					},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, true)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored,
				clientMock.ServiceLister,
				clientMock.EndpointsLister,
				clientMock.PodLister,
				clientMock.TrafficTargetLister,
				clientMock.HTTPRouteGroupLister,
				clientMock.TCPRouteLister,
				clientMock.TrafficSplitLister)
			config, err := provider.BuildConfig()
			assert.Equal(t, test.expected, config)
			if test.endpointsError || test.serviceError {
				assert.Error(t, err)
			}
		})
	}
}
