package base

import (
	"testing"

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
