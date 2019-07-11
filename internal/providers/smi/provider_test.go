package smi

import (
	"testing"

	"github.com/containous/i3o/internal/k8s"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	// splitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	"github.com/stretchr/testify/assert"
	// corev1 "k8s.io/api/core/v1"
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
	clientMock := k8s.NewClientMock("get_traffic_targets.yaml")
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
