package smi

import (
	"testing"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/message"
	"github.com/containous/traefik/pkg/config"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildRuleSnippetFromServiceAndMatch(t *testing.T) {
	provider := New(nil, k8s.ServiceTypeHTTP)

	testCases := []struct {
		desc     string
		expected string
		match    specsv1alpha1.HTTPMatch
	}{
		{
			desc:     "method and regex in match",
			expected: "(PathPrefix(`/foo`) && Methods(GET,POST) && (Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)))",
			match: specsv1alpha1.HTTPMatch{
				Name:      "test",
				Methods:   []string{"GET", "POST"},
				PathRegex: "/foo",
			},
		},
		{
			desc:     "method only in match",
			expected: "(Methods(GET,POST) && (Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)))",
			match: specsv1alpha1.HTTPMatch{
				Name:    "test",
				Methods: []string{"GET", "POST"},
			},
		},
		{
			desc:     "prefix only in match",
			expected: "(PathPrefix(`/foo`) && (Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)))",
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
	provider := New(clientMock, k8s.ServiceTypeHTTP)

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
		expected         *config.Router
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
			expected: &config.Router{
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
				Rule:        "((PathPrefix(`/metrics`) && Methods(GET) && (Host(`test.default.traefik.mesh`) || Host(`10.0.0.1`))))",
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
			expected: &config.Router{
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
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
			expected: &config.Router{
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
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
			expected: &config.Router{
				EntryPoints: []string{"ingress-81"},
				Service:     "example",
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
			provider := New(clientMock, k8s.ServiceTypeHTTP)

			actual := provider.buildRouterFromTrafficTarget(test.serviceName, test.serviceNamespace, test.serviceIP, test.trafficTarget, test.port, test.key)
			assert.Equal(t, test.expected, actual)

		})
	}
}

func TestGetServiceMode(t *testing.T) {
	provider := New(nil, k8s.ServiceTypeHTTP)

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
		desc             string
		serviceName      string
		serviceNamespace string
		trafficTargets   []*accessv1alpha1.TrafficTarget
		expected         []*accessv1alpha1.TrafficTarget
		endpointsError   bool
		podError         bool
	}{
		{
			desc:             "traffictarget destination in different namespace",
			serviceName:      "demo-service",
			serviceNamespace: metav1.NamespaceDefault,
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
			desc:             "valid traffictarget found",
			serviceName:      "demo-service",
			serviceNamespace: metav1.NamespaceDefault,
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
			desc:             "endpoints error",
			serviceName:      "demo-service",
			serviceNamespace: metav1.NamespaceDefault,
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
			expected:       nil,
			endpointsError: true,
		},
		{
			desc:             "endpoints don't exist",
			serviceName:      "demo-api",
			serviceNamespace: metav1.NamespaceDefault,
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
		{
			desc:             "no subset match",
			serviceName:      "demo-service",
			serviceNamespace: metav1.NamespaceDefault,
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
			desc:             "pod error",
			serviceName:      "demo-service",
			serviceNamespace: metav1.NamespaceDefault,
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
			desc:             "pod doesnt exist error",
			serviceName:      "demo-service-missing-pod",
			serviceNamespace: metav1.NamespaceDefault,
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

			if test.endpointsError {
				clientMock.EnableEndpointsError()
			}
			if test.podError {
				clientMock.EnablePodError()
			}

			provider := New(clientMock, k8s.ServiceTypeHTTP)

			actual := provider.getApplicableTrafficTargets(test.serviceName, test.serviceNamespace, test.trafficTargets)
			assert.Equal(t, test.expected, actual)

		})
	}
}

func TestBuildServiceFromTrafficTarget(t *testing.T) {
	testCases := []struct {
		desc          string
		endpoints     *corev1.Endpoints
		trafficTarget *accessv1alpha1.TrafficTarget
		expected      *config.Service
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
			expected: &config.Service{
				LoadBalancer: &config.LoadBalancerService{
					PassHostHeader: true,
					Servers: []config.Server{
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
			expected: &config.Service{
				LoadBalancer: &config.LoadBalancerService{
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
			expected: &config.Service{
				LoadBalancer: &config.LoadBalancerService{
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
			expected: &config.Service{
				LoadBalancer: &config.LoadBalancerService{
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

			provider := New(clientMock, k8s.ServiceTypeHTTP)

			actual := provider.buildServiceFromTrafficTarget(test.endpoints, test.trafficTarget)
			assert.Equal(t, test.expected, actual)
		})
	}

}

func TestGroupTrafficTargetsByDestination(t *testing.T) {
	provider := New(nil, k8s.ServiceTypeHTTP)

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
		provided       *config.Configuration
		expected       *config.Configuration
		endpointsError bool
		serviceError   bool
	}{
		{
			desc: "simple configuration build with empty event",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
		},
		{
			desc: "simple configuration build with HTTP service event",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers: map[string]*config.Router{
						"5bb66e727779b5ba3112d69259160957be7f58ce2caf1f9ec0d42c039a7b8ec9": {
							EntryPoints: []string{"ingress-5000"},
							Rule:        "((PathPrefix(`/metrics`) && Methods(GET) && (Host(`demo-service.default.traefik.mesh`) || Host(`10.1.0.1`))))",
							Service:     "5bb66e727779b5ba3112d69259160957be7f58ce2caf1f9ec0d42c039a7b8ec9",
						},
					},
					Services: map[string]*config.Service{
						"5bb66e727779b5ba3112d69259160957be7f58ce2caf1f9ec0d42c039a7b8ec9": {
							LoadBalancer: &config.LoadBalancerService{
								PassHostHeader: true,
								Servers: []config.Server{
									{
										URL: "http://10.1.1.50:50",
									},
								},
							},
						},
					},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
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
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers: map[string]*config.Router{
						"7f2af3b9b8c325734be45787c2167ed9081474e3dd74cb83630daf3512549952": {
							EntryPoints: []string{"ingress-5000"},
							Rule:        "((PathPrefix(`/metrics`) && Methods(GET) && (Host(`demo-test.default.traefik.mesh`) || Host(`10.1.0.1`))))",
							Service:     "7f2af3b9b8c325734be45787c2167ed9081474e3dd74cb83630daf3512549952",
						},
					},
					Services: map[string]*config.Service{
						"7f2af3b9b8c325734be45787c2167ed9081474e3dd74cb83630daf3512549952": {
							LoadBalancer: &config.LoadBalancerService{
								PassHostHeader: true,
								Servers: []config.Server{
									{
										URL: "http://10.1.1.50:50",
									},
								},
							},
						},
					},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
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
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
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
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
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
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
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
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
				TCP: &config.TCPConfiguration{
					Routers:  map[string]*config.TCPRouter{},
					Services: map[string]*config.TCPService{},
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

			provider := New(clientMock, k8s.ServiceTypeHTTP)
			provider.BuildConfiguration(test.event, test.provided)
			assert.Equal(t, test.expected, test.provided)
		})
	}
}
