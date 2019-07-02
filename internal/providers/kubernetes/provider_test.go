package kubernetes

import (
	"testing"

	"github.com/containous/traefik/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildRouterFromService(t *testing.T) {
	testCases := []struct {
		desc     string
		service  *corev1.Service
		expected *config.Router
	}{
		{
			desc: "",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "foo",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.0.0.1",
				},
			},
			expected: &config.Router{
				Rule: "Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)",
			},
		},
	}

	provider := New(nil)

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := provider.buildRouterFromService(test.service)

			assert.Equal(t, test.expected, actual)
		})
	}

}
