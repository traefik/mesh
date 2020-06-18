package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTopology_ResolveServicePort(t *testing.T) {
	tests := []struct {
		desc           string
		svcPort        corev1.ServicePort
		containerPorts []corev1.ContainerPort
		expPort        int32
		expResult      bool
	}{
		{
			desc: "should return the service TargetPort if it as Int",
			svcPort: corev1.ServicePort{
				TargetPort: intstr.FromInt(3000),
			},
			expPort:   3000,
			expResult: true,
		},
		{
			desc: "should return false if the service TargetPort is a String and the container port list is empty",
			svcPort: corev1.ServicePort{
				TargetPort: intstr.FromString("foo"),
			},
			expPort:   0,
			expResult: false,
		},
		{
			desc: "should return false if the service TargetPort is a String and it cannot be resolved",
			svcPort: corev1.ServicePort{
				TargetPort: intstr.FromString("foo"),
				Protocol:   corev1.ProtocolTCP,
			},
			containerPorts: []corev1.ContainerPort{
				{
					Name:          "bar",
					ContainerPort: 3000,
				},
				{
					Name:          "foo",
					Protocol:      corev1.ProtocolUDP,
					ContainerPort: 3000,
				},
			},
			expPort:   0,
			expResult: false,
		},
		{
			desc: "should return true and the resolved service TargetPort",
			svcPort: corev1.ServicePort{
				TargetPort: intstr.FromString("foo"),
				Protocol:   corev1.ProtocolUDP,
			},
			containerPorts: []corev1.ContainerPort{
				{
					Name:          "foo",
					Protocol:      corev1.ProtocolUDP,
					ContainerPort: 3000,
				},
			},
			expPort:   3000,
			expResult: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			port, result := ResolveServicePort(test.svcPort, test.containerPorts)

			assert.Equal(t, test.expResult, result)
			assert.Equal(t, test.expPort, port)
		})
	}
}
