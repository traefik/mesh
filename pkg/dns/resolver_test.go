package dns

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/mesh/v2/pkg/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
)

func TestShadowServiceResolver_LookupFQDN(t *testing.T) {
	tests := []struct {
		desc          string
		fqdn          string
		shadowService *v1.Service
		expIP         net.IP
		expErr        bool
	}{
		{
			desc:          "should return an error if shadow service does not exist",
			fqdn:          "foo.default.traefik.mesh.",
			shadowService: &v1.Service{},
			expErr:        true,
		},
		{
			desc: "should return the shadow service ClusterIP corresponding to the given FQDN",
			fqdn: "whoami.default.traefik.mesh.",
			shadowService: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shadow-svc-247b8d4abd40affb14cc82edca56b2c7",
					Namespace: "traefik-mesh",
					Labels: map[string]string{
						k8s.LabelServiceName:      "whoami",
						k8s.LabelServiceNamespace: "default",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.10.10.10",
				},
			},
			expIP: net.ParseIP("10.10.10.10"),
		},
		{
			desc:   "should return an error if there is a hash collision",
			fqdn:   "whoami.default.traefik.mesh.",
			expErr: true,
			shadowService: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shadow-svc-247b8d4abd40affb14cc82edca56b2c7",
					Namespace: "traefik-mesh",
					Labels: map[string]string{
						k8s.LabelServiceName:      "whoami",
						k8s.LabelServiceNamespace: "whoami",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			serviceLister := newFakeK8sClient(t, test.shadowService)
			resolver := NewShadowServiceResolver("traefik.mesh", "traefik-mesh", serviceLister)

			ip, err := resolver.LookupFQDN(test.fqdn)

			if test.expErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expIP, ip)
		})
	}
}

func TestShadowServiceResolver_parseNamespaceAndName(t *testing.T) {
	tests := []struct {
		desc         string
		fqdn         string
		expNamespace string
		expName      string
		expErr       bool
	}{
		{
			desc:         "should return the namespace and name",
			fqdn:         "name.namespace.traefik.mesh.",
			expNamespace: "namespace",
			expName:      "name",
		},
		{
			desc:   "should return an error if the FQDN is malformed",
			fqdn:   "namespace.traefik.mesh.",
			expErr: true,
		},
		{
			desc:   "should return an error if FQDN is not a subdomain of the configured domain",
			fqdn:   "name.namespace.traefik.local.",
			expErr: true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			serviceLister := newFakeK8sClient(t)
			resolver := NewShadowServiceResolver("traefik.mesh", "traefik-mesh", serviceLister)

			namespace, name, err := resolver.parseNamespaceAndName(test.fqdn)

			if test.expErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expNamespace, namespace)
			assert.Equal(t, test.expName, name)
		})
	}
}

func newFakeK8sClient(t *testing.T, objects ...runtime.Object) listers.ServiceLister {
	client := fake.NewSimpleClientset(objects...)

	informerFactory := informers.NewSharedInformerFactory(client, 5*time.Minute)
	serviceLister := informerFactory.Core().V1().Services().Lister()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	informerFactory.Start(ctx.Done())

	for typ, ok := range informerFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			require.NoError(t, fmt.Errorf("timed out waiting for controller caches to sync: %s", typ))
		}
	}

	return serviceLister
}
