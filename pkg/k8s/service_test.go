package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServicesContains(t *testing.T) {
	testCases := []struct {
		desc     string
		services Services
		value    Service
		expected bool
	}{
		{
			desc:     "empty services",
			services: Services{},
			value: Service{
				Name:      "foo",
				Namespace: "bar",
			},
			expected: false,
		},
		{
			desc: "element in services",
			services: Services{
				{
					Name:      "foo",
					Namespace: "bar",
				},
			},
			value: Service{
				Name:      "foo",
				Namespace: "bar",
			},
			expected: true,
		},
		{
			desc: "element not in services",
			services: Services{
				{
					Name:      "test",
					Namespace: "test",
				},
			},
			value: Service{
				Name:      "foo",
				Namespace: "bar",
			},
			expected: false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := test.services.Contains(test.value.Name, test.value.Namespace)
			assert.Equal(t, test.expected, actual)
		})
	}
}
