package base

import (
	"testing"

	"github.com/containous/maesh/internal/k8s"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetEndpointsFromList(t *testing.T) {
	testCases := []struct {
		desc      string
		provided  []*corev1.Endpoints
		name      string
		namespace string
		expected  *corev1.Endpoints
	}{
		{
			desc:      "empty list",
			provided:  []*corev1.Endpoints{},
			name:      "foo",
			namespace: "bar",
			expected:  nil,
		},
		{
			desc: "match in list",
			provided: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fifi",
						Namespace: "fufu",
					},
				},
			},
			name:      "foo",
			namespace: "bar",
			expected: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
		},
		{
			desc: "no match in list",
			provided: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bar",
						Namespace: "bar",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fifi",
						Namespace: "fufu",
					},
				},
			},
			name:      "foo",
			namespace: "bar",
			expected:  nil,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := GetEndpointsFromList(test.name, test.namespace, test.provided)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetScheme(t *testing.T) {
	testCases := []struct {
		desc        string
		annotations map[string]string
		expected    string
	}{
		{
			desc:     "empty annotations",
			expected: k8s.SchemeHTTP,
		},
		{
			desc: "Not exist",
			annotations: map[string]string{
				"not_exist": "not_exist",
			},
			expected: k8s.SchemeHTTP,
		},
		{
			desc: "HTTP exist",
			annotations: map[string]string{
				k8s.AnnotationScheme: k8s.SchemeHTTP,
			},
			expected: k8s.SchemeHTTP,
		},
		{
			desc: "H2c exist",
			annotations: map[string]string{
				k8s.AnnotationScheme: k8s.SchemeH2c,
			},
			expected: k8s.SchemeH2c,
		},
		{
			desc: "Unknown scheme",
			annotations: map[string]string{
				k8s.AnnotationScheme: "powpow",
			},
			expected: k8s.SchemeHTTP,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := GetScheme(test.annotations)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetServiceType(t *testing.T) {
	testCases := []struct {
		desc        string
		annotations map[string]string
		expected    string
	}{
		{
			desc:     "empty annotations",
			expected: k8s.ServiceTypeHTTP,
		},
		{
			desc: "Not exist",
			annotations: map[string]string{
				"not_exist": "not_exist",
			},
			expected: k8s.ServiceTypeHTTP,
		},
		{
			desc: "HTTP exist",
			annotations: map[string]string{
				k8s.AnnotationServiceType: k8s.ServiceTypeHTTP,
			},
			expected: k8s.ServiceTypeHTTP,
		},
		{
			desc: "TCP exist",
			annotations: map[string]string{
				k8s.AnnotationServiceType: k8s.ServiceTypeTCP,
			},
			expected: k8s.ServiceTypeTCP,
		},
		{
			desc: "Unknown scheme",
			annotations: map[string]string{
				k8s.AnnotationServiceType: "powpow",
			},
			expected: k8s.ServiceTypeHTTP,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := GetServiceMode(test.annotations, k8s.ServiceTypeHTTP)
			assert.Equal(t, test.expected, actual)
		})
	}
}
