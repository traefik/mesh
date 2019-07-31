package smi

import (
	"testing"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/message"
	"github.com/containous/traefik/pkg/config/dynamic"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const meshNamespace string = "maesh"

func TestBuildRuleSnippetFromServiceAndMatch(t *testing.T) {
	provider := New(nil, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))

	testCases := []struct {
		desc     string
		expected string
		match    specsv1alpha1.HTTPMatch
	}{
		{
			desc:     "method and regex in match",
			expected: "(PathPrefix(`/foo`) && Method(`GET`,`POST`) && (Host(`test.foo.maesh`) || Host(`10.0.0.1`)))",
			match: specsv1alpha1.HTTPMatch{
				Name:      "test",
				Methods:   []string{"GET", "POST"},
				PathRegex: "/foo",
			},
		},
		{
			desc:     "method only in match",
			expected: "(Method(`GET`,`POST`) && (Host(`test.foo.maesh`) || Host(`10.0.0.1`)))",
			match: specsv1alpha1.HTTPMatch{
				Name:    "test",
				Methods: []string{"GET", "POST"},
			},
		},
		{
			desc:     "prefix only in match",
			expected: "(PathPrefix(`/foo`) && (Host(`test.foo.maesh`) || Host(`10.0.0.1`)))",
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
	clientMock := k8s.NewClientMock("mock.yaml")
	provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))

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
	actual := provider.getTrafficTargetsWithDestinationInNamespace("foo")
	assert.Equal(t, expected, actual)

	clientMock.EnableTrafficTargetError()

	var newExpected []*accessv1alpha1.TrafficTarget
	newActual := provider.getTrafficTargetsWithDestinationInNamespace("foo")
	assert.Equal(t, newExpected, newActual)
}

func TestBuildRouterFromTrafficTarget(t *testing.T) {
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
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
				Rule:        "((PathPrefix(`/metrics`) && Method(`GET`) && (Host(`test.default.maesh`) || Host(`10.0.0.1`))))",
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
				EntryPoints: []string{"ingress-81"},
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
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
				Middlewares: []string{"block-all"},
			},
		},
		{
			desc:             "simple router with HTTPRouteGroup error",
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
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
				Middlewares: []string{"block-all"},
			},
			httpError: true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewClientMock("mock.yaml")
			if test.httpError {
				clientMock.EnableHTTPRouteGroupError()
			}
			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))
			middleware := "block-all"
			actual := provider.buildRouterFromTrafficTarget(test.serviceName, test.serviceNamespace, test.serviceIP, test.trafficTarget, test.port, test.key, middleware)
			assert.Equal(t, test.expected, actual)

		})
	}
}

func TestGetServiceMode(t *testing.T) {
	provider := New(nil, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))

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
		podError       bool
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
			desc: "pod error",
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
			expected: nil,
			podError: true,
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
			clientMock := k8s.NewClientMock("mock.yaml")

			if test.podError {
				clientMock.EnablePodError()
			}

			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))

			actual := provider.getApplicableTrafficTargets(test.endpoints, test.trafficTargets)
			assert.Equal(t, test.expected, actual)

		})
	}
}

func TestBuildServiceFromTrafficTarget(t *testing.T) {
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
				LoadBalancer: &dynamic.LoadBalancerService{
					PassHostHeader: true,
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
				LoadBalancer: &dynamic.LoadBalancerService{
					PassHostHeader: true,
					Servers:        nil,
				},
			},
		},
		{
			desc: "pod error",
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
				LoadBalancer: &dynamic.LoadBalancerService{
					PassHostHeader: true,
					Servers:        nil,
				},
			},
			podError: true,
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
				LoadBalancer: &dynamic.LoadBalancerService{
					PassHostHeader: true,
					Servers:        nil,
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewClientMock("mock.yaml")

			if test.podError {
				clientMock.EnablePodError()
			}

			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))

			actual := provider.buildServiceFromTrafficTarget(test.endpoints, test.trafficTarget)
			assert.Equal(t, test.expected, actual)
		})
	}

}

func TestGroupTrafficTargetsByDestination(t *testing.T) {
	provider := New(nil, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))

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
		event          message.Message
		provided       *dynamic.Configuration
		expected       *dynamic.Configuration
		endpointsError bool
		serviceError   bool
	}{
		{
			desc: "simple configuration build with empty event",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
		},
		{
			desc: "simple configuration build with HTTP service event",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"demo-service-default-80-api-service-metrics-default-5bb66e727779b5b": {
							EntryPoints: []string{"ingress-5000"},
							Rule:        "((PathPrefix(`/metrics`) && Method(`GET`) && (Host(`demo-service.default.maesh`) || Host(`10.1.0.1`))))",
							Service:     "demo-service-default-80-api-service-metrics-default-5bb66e727779b5b",
							Middlewares: []string{"api-service-metrics-default-demo-service-default-80-api-service-metrics-default-5bb66e727779b5b-whitelist"},
						},
					},
					Services: map[string]*dynamic.Service{
						"demo-service-default-80-api-service-metrics-default-5bb66e727779b5b": {
							LoadBalancer: &dynamic.LoadBalancerService{
								PassHostHeader: true,
								Servers: []dynamic.Server{
									{
										URL: "http://10.1.1.50:50",
									},
								},
							},
						},
					},
					Middlewares: map[string]*dynamic.Middleware{
						"api-service-metrics-default-demo-service-default-80-api-service-metrics-default-5bb66e727779b5b-whitelist": {
							IPWhiteList: &dynamic.IPWhiteList{
								SourceRange: []string{"10.4.3.2"},
							},
						},
					},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "demo-service",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.1.0.1",
						Ports: []corev1.ServicePort{
							{
								Name:     "test",
								Port:     80,
								Protocol: "TCP",
							},
						},
					},
				},
				Action: message.TypeCreated,
			},
		},
		{
			desc: "simple configuration build with HTTP endpoint event",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"demo-test-default-80-api-service-metrics-default-7f2af3b9b8c3257": {
							EntryPoints: []string{"ingress-5000"},
							Rule:        "((PathPrefix(`/metrics`) && Method(`GET`) && (Host(`demo-test.default.maesh`) || Host(`10.1.0.1`))))",
							Service:     "demo-test-default-80-api-service-metrics-default-7f2af3b9b8c3257",
							Middlewares: []string{"api-service-metrics-default-demo-test-default-80-api-service-metrics-default-7f2af3b9b8c3257-whitelist"},
						},
					},
					Services: map[string]*dynamic.Service{
						"demo-test-default-80-api-service-metrics-default-7f2af3b9b8c3257": {
							LoadBalancer: &dynamic.LoadBalancerService{
								PassHostHeader: true,
								Servers: []dynamic.Server{
									{
										URL: "http://10.1.1.50:50",
									},
								},
							},
						},
					},
					Middlewares: map[string]*dynamic.Middleware{
						"api-service-metrics-default-demo-test-default-80-api-service-metrics-default-7f2af3b9b8c3257-whitelist": {
							IPWhiteList: &dynamic.IPWhiteList{
								SourceRange: []string{"10.4.3.2"},
							},
						},
					},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "demo-test",
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
				Action: message.TypeUpdated,
			},
		},
		{
			desc: "simple configuration build with endpoint error",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "demo-service",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.1.0.1",
						Ports: []corev1.ServicePort{
							{
								Name:     "test",
								Port:     80,
								Protocol: "TCP",
							},
						},
					},
				},
				Action: message.TypeCreated,
			},
			endpointsError: true,
		},
		{
			desc: "simple configuration build with endpoints don't exist",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "demo-service-foobar",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.1.0.1",
						Ports: []corev1.ServicePort{
							{
								Name:     "test",
								Port:     80,
								Protocol: "TCP",
							},
						},
					},
				},
				Action: message.TypeCreated,
			},
		},
		{
			desc: "simple configuration build with service error",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "demo-test",
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
				Action: message.TypeUpdated,
			},
			serviceError: true,
		},
		{
			desc: "simple configuration build with service doesn't exist",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:     map[string]*dynamic.Router{},
					Services:    map[string]*dynamic.Service{},
					Middlewares: map[string]*dynamic.Middleware{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "demo-test-foobar",
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
				Action: message.TypeUpdated,
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewClientMock("mock.yaml")
			if test.endpointsError {
				clientMock.EnableEndpointsError()
			}
			if test.serviceError {
				clientMock.EnableServiceError()
			}

			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, k8s.NewIgnored(meshNamespace))
			provider.BuildConfiguration(test.event, test.provided)
			assert.Equal(t, test.expected, test.provided)
		})
	}
}
