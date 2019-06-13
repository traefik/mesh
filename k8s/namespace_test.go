package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespacesContains(t *testing.T) {
	testCases := []struct {
		desc       string
		namespaces Namespaces
		value      string
		expected   bool
	}{
		{
			desc:       "empty namespaces",
			namespaces: Namespaces{},
			value:      "",
			expected:   false,
		},
		{
			desc:       "element in namespaces",
			namespaces: Namespaces{"powpow"},
			value:      "powpow",
			expected:   true,
		},
		{
			desc:       "element not in namespaces",
			namespaces: Namespaces{"powpow"},
			value:      "powpow1",
			expected:   false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := test.namespaces.Contains(test.value)
			assert.Equal(t, test.expected, actual)
		})
	}
}
