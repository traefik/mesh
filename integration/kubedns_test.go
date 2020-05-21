package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubeDNSSuite.
type KubeDNSSuite struct{ BaseSuite }

func (s *KubeDNSSuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"containous/whoami:v1.0.1",
		"coredns/coredns:1.6.3",
		"gcr.io/google_containers/k8s-dns-kube-dns-amd64:1.14.7",
		"gcr.io/google_containers/k8s-dns-dnsmasq-nanny-amd64:1.14.7",
		"gcr.io/google_containers/k8s-dns-sidecar-amd64:1.14.7",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForKubeDNS(c)
	s.WaitForCoreDNS(c)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
}

func (s *KubeDNSSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *KubeDNSSuite) TestKubeDNSDig(c *check.C) {
	cmd := s.startMaeshBinaryCmd(c, false, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	s.digHost(c, pod.Name, pod.Namespace, "whoami.whoami.maesh")
}
