package dns

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/mesh/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckDNSProvider(t *testing.T) {
	tests := []struct {
		desc        string
		mockFile    string
		expProvider Provider
		expErr      bool
	}{
		{
			desc:        "KubeDNS",
			mockFile:    "checkdnsprovider_kubedns.yaml",
			expProvider: KubeDNS,
			expErr:      false,
		},
		{
			desc:        "CoreDNS supported version",
			mockFile:    "checkdnsprovider_supported_version.yaml",
			expProvider: CoreDNS,
			expErr:      false,
		},
		{
			desc:        "CoreDNS supported version with deployment label",
			mockFile:    "checkdnsprovider_supported_version_with_label.yaml",
			expProvider: CoreDNS,
			expErr:      false,
		},
		{
			desc:        "CoreDNS supported version on RKE",
			mockFile:    "checkdnsprovider_supported_version_rke.yaml",
			expProvider: CoreDNS,
			expErr:      false,
		},
		{
			desc:        "CoreDNS supported version with suffix",
			mockFile:    "checkdnsprovider_supported_version_suffix.yaml",
			expProvider: CoreDNS,
			expErr:      false,
		},
		{
			desc:        "CoreDNS supported min version with suffix",
			mockFile:    "checkdnsprovider_supported_min_version_suffix.yaml",
			expProvider: CoreDNS,
			expErr:      false,
		},
		{
			desc:        "CoreDNS unsupported version",
			mockFile:    "checkdnsprovider_unsupported_version.yaml",
			expProvider: UnknownDNS,
			expErr:      true,
		},
		{
			desc:        "No known DNS provider",
			mockFile:    "checkdnsprovider_no_provider.yaml",
			expProvider: UnknownDNS,
			expErr:      true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(test.mockFile)

			log := logrus.New()
			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			provider, err := client.CheckDNSProvider(ctx)
			if test.expErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expProvider, provider)
		})
	}
}

func TestConfigureCoreDNS(t *testing.T) {
	tests := []struct {
		desc         string
		mockFile     string
		resourceName string
		expCorefile  string
		expCustoms   map[string]string
		expErr       bool
		expRestart   bool
	}{
		{
			desc:         "First time config of CoreDNS",
			mockFile:     "configurecoredns_not_patched.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			expRestart:   true,
		},
		{
			desc:         "First time config of CoreDNS on RKE",
			mockFile:     "configurecoredns_not_patched_rke.yaml",
			resourceName: "rke2-coredns-rke2-coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			expRestart:   true,
		},
		{
			desc:         "Already patched CoreDNS config",
			mockFile:     "configurecoredns_already_patched.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			expRestart:   false,
		},
		{
			desc:         "Missing Corefile configmap",
			mockFile:     "configurecoredns_missing_configmap.yaml",
			resourceName: "coredns",
			expErr:       true,
			expRestart:   false,
		},
		{
			desc:         "First time config of CoreDNS custom",
			mockFile:     "configurecoredns_custom_not_patched.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
			expCustoms: map[string]string{
				"maesh.server":        "\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
				"traefik.mesh.server": "\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			},
			expRestart: true,
		},
		{
			desc:         "Already patched CoreDNS custom config",
			mockFile:     "configurecoredns_custom_already_patched.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
			expCustoms: map[string]string{
				"maesh.server":        "#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
				"traefik.mesh.server": "#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			},
			expRestart: false,
		},
		{
			desc:         "Config of CoreDNS 1.7",
			mockFile:     "configurecoredns_17.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			expRestart:   true,
		},
		{
			desc:         "Config of CoreDNS 1.7 with version suffix",
			mockFile:     "configurecoredns_17_suffix.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			expRestart:   true,
		},
		{
			desc:         "CoreDNS 1.7 already patched for an older version of CoreDNS",
			mockFile:     "configurecoredns_17_already_patched.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			expRestart:   true,
		},
		{
			desc:         "CoreDNS 1.7 custom config already patched for an older version of CoreDNS",
			mockFile:     "configurecoredns_17_custom_already_patched.yaml",
			resourceName: "coredns",
			expErr:       false,
			expCorefile:  ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
			expCustoms: map[string]string{
				"maesh.server":        "\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
				"traefik.mesh.server": "\n#### Begin Traefik Mesh Block\ntraefik.mesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.traefik.mesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.traefik.mesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Traefik Mesh Block\n",
			},
			expRestart: true,
		},
		{
			desc:         "Missing CoreDNS deployment",
			mockFile:     "configurecoredns_missing_deployment.yaml",
			resourceName: "coredns",
			expErr:       true,
			expRestart:   false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(test.mockFile)

			log := logrus.New()
			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.ConfigureCoreDNS(ctx, metav1.NamespaceSystem, "titi", "toto")
			if test.expErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, test.resourceName, metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expCorefile, cfgMap.Data["Corefile"])

			if len(test.expCustoms) > 0 {
				var customCfgMap *corev1.ConfigMap

				customCfgMap, err = k8sClient.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "coredns-custom", metav1.GetOptions{})
				require.NoError(t, err)

				for key, value := range test.expCustoms {
					assert.Equal(t, value, customCfgMap.Data[key])
				}
			}

			coreDNSDeployment, err := k8sClient.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, test.resourceName, metav1.GetOptions{})
			require.NoError(t, err)

			restarted := coreDNSDeployment.Spec.Template.Annotations["traefik-mesh-hash"] != ""
			assert.Equal(t, test.expRestart, restarted)
		})
	}
}

func TestConfigureKubeDNS(t *testing.T) {
	tests := []struct {
		desc           string
		mockFile       string
		expStubDomains string
		expErr         bool
	}{
		{
			desc:     "should return an error if kube-dns deployment does not exist",
			mockFile: "configurekubedns_missing_deployment.yaml",
			expErr:   true,
		},
		{
			desc:           "should add stubdomains config in kube-dns configmap",
			mockFile:       "configurekubedns_not_patched.yaml",
			expStubDomains: `{"maesh":["1.2.3.4"],"traefik.mesh":["1.2.3.4"]}`,
		},
		{
			desc:           "should replace stubdomains config in kube-dns configmap",
			mockFile:       "configurekubedns_already_patched.yaml",
			expStubDomains: `{"maesh":["1.2.3.4"],"traefik.mesh":["1.2.3.4"]}`,
		},
		{
			desc:           "should create optional kube-dns configmap and add stubdomains config",
			mockFile:       "configurekubedns_optional_configmap.yaml",
			expStubDomains: `{"maesh":["1.2.3.4"],"traefik.mesh":["1.2.3.4"]}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(test.mockFile)

			log := logrus.New()
			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.ConfigureKubeDNS(ctx, "cluster.local", "traefik-mesh")
			if test.expErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expStubDomains, cfgMap.Data["stubDomains"])
		})
	}
}

func TestRestoreCoreDNS(t *testing.T) {
	tests := []struct {
		desc        string
		mockFile    string
		cfgMapName  string
		hasCustom   bool
		expCorefile string
	}{
		{
			desc:        "CoreDNS config patched",
			mockFile:    "restorecoredns_patched.yaml",
			cfgMapName:  "coredns",
			expCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n\n# This is test data that must be present\n",
		},
		{
			desc:        "CoreDNS config patched on RKE",
			mockFile:    "restorecoredns_patched_rke.yaml",
			cfgMapName:  "rke2-coredns-rke2-coredns",
			expCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n\n# This is test data that must be present\n",
		},
		{
			desc:        "CoreDNS config not patched",
			mockFile:    "restorecoredns_not_patched.yaml",
			cfgMapName:  "coredns",
			expCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
		},
		{
			desc:        "CoreDNS custom config patched",
			mockFile:    "restorecoredns_custom_patched.yaml",
			cfgMapName:  "coredns",
			hasCustom:   true,
			expCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n",
		},
		{
			desc:        "CoreDNS custom config not patched",
			mockFile:    "restorecoredns_custom_not_patched.yaml",
			cfgMapName:  "coredns",
			hasCustom:   true,
			expCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(test.mockFile)

			log := logrus.New()
			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.RestoreCoreDNS(ctx)
			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, test.cfgMapName, metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expCorefile, cfgMap.Data["Corefile"])

			if test.hasCustom {
				customCfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "coredns-custom", metav1.GetOptions{})
				require.NoError(t, err)

				_, exists := customCfgMap.Data["maesh.server"]
				assert.False(t, exists)

				_, exists = customCfgMap.Data["test.server"]
				assert.True(t, exists)
			}
		})
	}
}

func TestRestoreKubeDNS(t *testing.T) {
	tests := []struct {
		desc           string
		mockFile       string
		expStubDomains string
	}{
		{
			desc:           "Not patched",
			mockFile:       "restorekubedns_not_patched.yaml",
			expStubDomains: "",
		},
		{
			desc:           "Already patched",
			mockFile:       "restorekubedns_already_patched.yaml",
			expStubDomains: `{"test":["5.6.7.8"]}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(test.mockFile)

			log := logrus.New()
			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.RestoreKubeDNS(ctx)
			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "kube-dns", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expStubDomains, cfgMap.Data["stubDomains"])
		})
	}
}
