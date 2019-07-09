package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIgnored(t *testing.T) {
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
			desc:      "ignored namespace",
			name:      "foo",
			namespace: metav1.NamespaceSystem,
			expected:  true,
		},
		{
			desc:      "ignored service",
			name:      "kubernetes",
			namespace: metav1.NamespaceDefault,
			expected:  true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			i := NewIgnored()
			actual := i.Ignored(test.name, test.namespace)
			assert.Equal(t, test.expected, actual)
		})
	}
}
