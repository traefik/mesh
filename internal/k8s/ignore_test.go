package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIgnored(t *testing.T) {
	meshNamespace := "maesh"
	testCases := []struct {
		desc      string
		name      string
		namespace string
		expected  bool
	}{
		{
			desc:      "empty ignored",
			name:      "",
			namespace: "",
			expected:  false,
		},
		{
			desc:      "ignored k8s default namespace",
			name:      "foo",
			namespace: metav1.NamespaceSystem,
			expected:  true,
		},
		{
			desc:      "ignored another namespace",
			name:      "foo",
			namespace: "someNamespace",
			expected:  true,
		},
		{
			desc:      "ignored service",
			name:      "kubernetes",
			namespace: metav1.NamespaceDefault,
			expected:  true,
		},
		{
			desc:      "ignored mesh service",
			name:      "omg",
			namespace: "maesh",
			expected:  true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			i := NewIgnored(meshNamespace, []string{"someNamespace"})
			actual := i.Ignored(test.name, test.namespace)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestWithoutMesh(t *testing.T) {
	meshNamespace := "maesh"
	testCases := []struct {
		desc      string
		name      string
		namespace string
		expected  bool
	}{
		{
			desc:      "empty ignored",
			name:      "",
			namespace: "",
			expected:  false,
		},
		{
			desc:      "ignored k8s default namespace",
			name:      "foo",
			namespace: metav1.NamespaceSystem,
			expected:  true,
		},
		{
			desc:      "ignored another namespace",
			name:      "foo",
			namespace: "someNamespace",
			expected:  true,
		},
		{
			desc:      "ignored service",
			name:      "kubernetes",
			namespace: metav1.NamespaceDefault,
			expected:  true,
		},
		{
			desc:      "ignored mesh service",
			name:      "omg",
			namespace: "maesh",
			expected:  false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			i := NewIgnored(meshNamespace, []string{"someNamespace"})
			i = i.WithoutMesh()
			actual := i.Ignored(test.name, test.namespace)
			assert.Equal(t, test.expected, actual)
		})
	}
}
