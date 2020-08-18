package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CoreDNSSuite.
type CoreDNSSuite struct{ BaseSuite }

func (s *CoreDNSSuite) SetUpSuite(c *check.C) {
	requiredImages := []image{
		{name: "containous/whoami:v1.0.1"},
		{name: "coredns/coredns:1.3.1"},
		{name: "coredns/coredns:1.4.0"},
		{name: "coredns/coredns:1.5.2"},
		{name: "coredns/coredns:1.6.3"},
		{name: "coredns/coredns:1.7.0"},
		{name: "giantswarm/tiny-tools:3.9"},
	}

	s.startk3s(c, requiredImages)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
	s.createResources(c, "testdata/smi/crds/")
}

func (s *CoreDNSSuite) TearDownSuite(_ *check.C) {
	s.stopK3s()
}

// TestCoreDNSLegacy tests specific version of CoreDNS which did not support the `ready` plugin. It checks that
// the prepare command successfully patches the Corefile or custom Corefile and that we can dig it.
func (s *CoreDNSSuite) TestCoreDNSLegacy(c *check.C) {
	versions := []string{"1.3.1", "1.4.0"}

	s.createResources(c, "testdata/coredns/coredns-legacy.yaml")
	s.waitForCoreDNS(c)

	for _, version := range versions {
		s.testCoreDNSVersion(c, version)
	}

	s.deleteResources(c, "testdata/coredns/coredns-legacy.yaml")
}

func (s *CoreDNSSuite) TestCoreDNS(c *check.C) {
	versions := []string{"1.5.2", "1.6.3", "1.7.0"}

	s.createResources(c, "testdata/coredns/coredns.yaml")
	s.waitForCoreDNS(c)

	for _, version := range versions {
		s.testCoreDNSVersion(c, version)
	}

	s.deleteResources(c, "testdata/coredns/coredns.yaml")
}

func (s *CoreDNSSuite) testCoreDNSVersion(c *check.C, version string) {
	c.Logf("Testing dig with CoreDNS %s", version)

	s.setCoreDNSVersion(c, version)

	cmd := s.startMaeshBinaryCmd(c, false, false)

	err := cmd.Start()
	c.Assert(err, checker.IsNil)

	defer s.stopMaeshBinary(c, cmd.Process)

	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	s.digHost(c, pod.Name, pod.Namespace, "whoami.whoami.maesh")
}

func (s *CoreDNSSuite) setCoreDNSVersion(c *check.C, version string) {
	ctx := context.Background()
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = 60 * time.Second

	err := backoff.Retry(safe.OperationWithRecover(func() error {
		// Get current coreDNS deployment.
		deployment, err := s.client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
		c.Assert(err, checker.IsNil)

		newDeployment := deployment.DeepCopy()
		c.Assert(len(newDeployment.Spec.Template.Spec.Containers), checker.Equals, 1)

		newDeployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("coredns/coredns:%s", version)

		return s.try.WaitUpdateDeployment(newDeployment, 10*time.Second)
	}), ebo)

	c.Assert(err, checker.IsNil)

	s.waitForCoreDNS(c)
}
