package integration

import (
	"time"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubeDNSSuite.
type KubeDNSSuite struct{ BaseSuite }

func (s *KubeDNSSuite) SetUpSuite(c *check.C) {
	requiredImages := []image{
		{repository: "containous/whoami", tag: "v1.0.1"},
		{repository: "coredns/coredns", tag: "1.6.3"},
		{repository: "gcr.io/google_containers/k8s-dns-kube-dns-amd64", tag: "1.14.7"},
		{repository: "gcr.io/google_containers/k8s-dns-dnsmasq-nanny-amd64", tag: "1.14.7"},
		{repository: "gcr.io/google_containers/k8s-dns-sidecar-amd64", tag: "1.14.7"},
	}

	s.startk3s(c, requiredImages)
	s.startAndWaitForKubeDNS(c)

	// Wait for our created coreDNS deployment in the maesh namespace.
	c.Assert(s.try.WaitReadyDeployment("coredns", maeshNamespace, 60*time.Second), checker.IsNil)

	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
	s.createResources(c, "testdata/smi/crds/")
}

func (s *KubeDNSSuite) TearDownSuite(_ *check.C) {
	s.stopK3s()
}

func (s *KubeDNSSuite) TestKubeDNSDig(c *check.C) {
	s.WaitForKubeDNS(c)

	cmd := s.startMaeshBinaryCmd(c, false, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	// We need to wait for kubeDNS again, as the pods will be restarted by prepare.
	s.WaitForKubeDNS(c)
	s.digHost(c, pod.Name, pod.Namespace, "whoami.whoami.maesh")
}
