package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIgnoredNamespace(t *testing.T) {
	testCases := []struct {
		desc      string
		namespace string
		expected  bool
	}{
		{
			desc:      "empty ignored",
			namespace: "",
			expected:  false,
		},
		{
			desc:      "not ignored namespace",
			namespace: "foo",
			expected:  false,
		},
		{
			desc:      "ignored namespace",
			namespace: "someNamespace",
			expected:  true,
		},
		{
			desc:      "ignored mesh namespace",
			namespace: "maesh",
			expected:  false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			i := NewIgnored()
			i.AddIgnoredNamespace("someNamespace")
			actual := i.IsIgnoredNamespace(test.namespace)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestIgnoredService(t *testing.T) {
	testCases := []struct {
		desc      string
		name      string
		namespace string
		app       string
		expected  bool
	}{
		{
			desc:      "empty ignored",
			name:      "",
			namespace: "",
			app:       "",
			expected:  false,
		},
		{
			desc:      "ignored service due to namespace",
			name:      "foo",
			namespace: "someNamespace",
			app:       "notignored",
			expected:  true,
		},
		{
			desc:      "explicit ignored service",
			name:      "foo",
			namespace: "bar",
			app:       "notignored",
			expected:  true,
		},
		{
			desc:      "ignored app",
			name:      "omg",
			namespace: "foo",
			app:       "ignoredapp",
			expected:  true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			i := NewIgnored()
			i.AddIgnoredNamespace("someNamespace")
			i.AddIgnoredService("foo", "bar")
			i.AddIgnoredApps("ignoredapp")
			actual := i.IsIgnoredService(test.name, test.namespace, test.app)
			assert.Equal(t, test.expected, actual)
		})
	}
}
