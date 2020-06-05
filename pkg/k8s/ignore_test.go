package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIgnoreWrapper_IsIgnored(t *testing.T) {
	testCases := []struct {
		desc string
		obj  metav1.Object
		want bool
	}{
		{
			desc: "object is ignored when namespace is ignored",
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "ignored-ns",
				},
			},
			want: true,
		},
		{
			desc: "object is ignored when app is ignored",
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "ns",
					Labels: map[string]string{
						"app": "ignored-app",
					},
				},
			},
			want: true,
		},
		{
			desc: "object is ignored when service is ignored",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ignored-svc",
					Namespace: "ns",
				},
				Spec: v1.ServiceSpec{
					Type: "ClusterIP",
				},
			},
			want: true,
		},
		{
			desc: "object is not ignored when it's a pod with the same name as an ignored service",
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ignored-svc",
					Namespace: "ns",
				},
			},
			want: false,
		},
		{
			desc: "object is ignored if it's a service of type ExternalName",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-a",
					Namespace: "ns",
				},
				Spec: v1.ServiceSpec{
					Type:         "ExternalName",
					ExternalName: "hello.com",
				},
			},
			want: true,
		},
		{
			desc: "pod is not ignored pods is doesn't match criteria",
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "ns",
				},
			},
			want: false,
		},
		{
			desc: "service is not ignored pods is doesn't match criteria",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
					Namespace: "ns",
				},
				Spec: v1.ServiceSpec{
					Type: "ClusterIP",
				},
			},
			want: false,
		},
	}
	i := NewIgnored()
	i.AddIgnoredNamespace("ignored-ns")
	i.AddIgnoredService("ignored-svc", "ns")
	i.AddIgnoredApps("ignored-app")

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			got := i.IsIgnored(test.obj)

			assert.Equal(t, test.want, got)
		})
	}
}
