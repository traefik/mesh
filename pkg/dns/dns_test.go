package dns

import (
	"context"
	"os"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckDNSProvider(t *testing.T) {
	tests := []struct {
		desc             string
		mockFile         string
		expectedProvider Provider
		expectedErr      bool
	}{
		{
			desc:             "CoreDNS supported version",
			mockFile:         "checkdnsprovider_supported_version.yaml",
			expectedProvider: CoreDNS,
			expectedErr:      false,
		},
		{
			desc:             "KubeDNS",
			mockFile:         "checkdnsprovider_kubedns.yaml",
			expectedProvider: KubeDNS,
			expectedErr:      false,
		},
		{
			desc:             "CoreDNS unsupported version",
			mockFile:         "checkdnsprovider_unsupported_version.yaml",
			expectedProvider: UnknownDNS,
			expectedErr:      true,
		},
		{
			desc:             "No known DNS provider",
			mockFile:         "checkdnsprovider_no_provider.yaml",
			expectedProvider: UnknownDNS,
			expectedErr:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			provider, err := client.CheckDNSProvider(ctx)
			if test.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expectedProvider, provider)
		})
	}
}

func TestConfigureCoreDNS(t *testing.T) {
	tests := []struct {
		desc             string
		mockFile         string
		expectedCorefile string
		expectedCustom   string
		expectedErr      bool
		expectedRestart  bool
	}{
		{
			desc:             "First time config of CoreDNS",
			mockFile:         "configurecoredns_not_patched.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  true,
		},
		{
			desc:             "Already patched CoreDNS config",
			mockFile:         "configurecoredns_already_patched.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  false,
		},
		{
			desc:            "Missing Corefile configmap",
			mockFile:        "configurecoredns_missing_configmap.yaml",
			expectedErr:     true,
			expectedRestart: false,
		},
		{
			desc:             "First time config of CoreDNS custom",
			mockFile:         "configurecoredns_custom_not_patched.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
			expectedCustom:   "\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  true,
		},
		{
			desc:             "Already patched CoreDNS custom config",
			mockFile:         "configurecoredns_custom_already_patched.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
			expectedCustom:   "#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  false,
		},
		{
			desc:             "Config of CoreDNS 1.7",
			mockFile:         "configurecoredns_17.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  true,
		},
		{
			desc:             "CoreDNS 1.7 already patched for an older version of CoreDNS",
			mockFile:         "configurecoredns_17_already_patched.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  true,
		},
		{
			desc:             "CoreDNS 1.7 custom config already patched for an older version of CoreDNS",
			mockFile:         "configurecoredns_17_custom_already_patched.yaml",
			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
			expectedCustom:   "\n#### Begin Maesh Block\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        \n        fallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n#### End Maesh Block\n",
			expectedRestart:  true,
		},
		{
			desc:            "Missing CoreDNS deployment",
			mockFile:        "configurecoredns_missing_deployment.yaml",
			expectedErr:     true,
			expectedRestart: false,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.ConfigureCoreDNS(ctx, "kube-system", "titi", "toto")
			if test.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedCorefile, cfgMap.Data["Corefile"])

			if len(test.expectedCustom) > 0 {
				var customCfgMap *corev1.ConfigMap

				customCfgMap, err = k8sClient.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns-custom", metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, test.expectedCustom, customCfgMap.Data["maesh.server"])
			}

			coreDNSDeployment, err := k8sClient.KubernetesClient().AppsV1().Deployments("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
			require.NoError(t, err)

			restarted := coreDNSDeployment.Spec.Template.Annotations["maesh-hash"] != ""
			assert.Equal(t, test.expectedRestart, restarted)
		})
	}
}

func TestConfigureKubeDNS(t *testing.T) {
	tests := []struct {
		desc                string
		mockFile            string
		expectedStubDomains string
		expectedErr         bool
	}{
		{
			desc:        "should return an error if kube-dns deployment does not exist",
			mockFile:    "configurekubedns_missing_deployment.yaml",
			expectedErr: true,
		},
		{
			desc:                "should add maesh stubdomain config in kube-dns configmap",
			mockFile:            "configurekubedns_not_patched.yaml",
			expectedStubDomains: `{"maesh":["1.2.3.4"]}`,
		},
		{
			desc:                "should replace maesh stubdomain config in kube-dns configmap",
			mockFile:            "configurekubedns_already_patched.yaml",
			expectedStubDomains: `{"maesh":["1.2.3.4"]}`,
		},
		{
			desc:                "should create optional kube-dns configmap and add maesh stubdomain config",
			mockFile:            "configurekubedns_optional_configmap.yaml",
			expectedStubDomains: `{"maesh":["1.2.3.4"]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.ConfigureKubeDNS(ctx, "cluster.local", "maesh")
			if test.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get(ctx, "kube-dns", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedStubDomains, cfgMap.Data["stubDomains"])
		})
	}
}

func TestRestoreCoreDNS(t *testing.T) {
	tests := []struct {
		desc             string
		mockFile         string
		hasCustom        bool
		expectedCorefile string
	}{
		{
			desc:             "CoreDNS config patched",
			mockFile:         "restorecoredns_patched.yaml",
			expectedCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n# This is test data that must be present\n",
		},
		{
			desc:             "CoreDNS config not patched",
			mockFile:         "restorecoredns_not_patched.yaml",
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
		},
		{
			desc:             "CoreDNS custom config patched",
			mockFile:         "restorecoredns_custom_patched.yaml",
			hasCustom:        true,
			expectedCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n",
		},
		{
			desc:             "CoreDNS custom config not patched",
			mockFile:         "restorecoredns_custom_not_patched.yaml",
			hasCustom:        true,
			expectedCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\n",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.RestoreCoreDNS(ctx)
			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedCorefile, cfgMap.Data["Corefile"])

			if test.hasCustom {
				customCfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns-custom", metav1.GetOptions{})
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
		desc                string
		mockFile            string
		expectedStubDomains string
	}{
		{
			desc:                "Not patched",
			mockFile:            "restorekubedns_not_patched.yaml",
			expectedStubDomains: "",
		},
		{
			desc:                "Already patched",
			mockFile:            "restorekubedns_already_patched.yaml",
			expectedStubDomains: `{"test":["5.6.7.8"]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			k8sClient := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := NewClient(log, k8sClient.KubernetesClient())

			err := client.RestoreKubeDNS(ctx)
			require.NoError(t, err)

			cfgMap, err := k8sClient.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get(ctx, "kube-dns", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedStubDomains, cfgMap.Data["stubDomains"])
		})
	}
}
