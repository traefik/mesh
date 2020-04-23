package provider_test

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/containous/maesh/pkg/event"
	mk8s "github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/provider"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type topologyBuilderMock func() (*topology.Topology, error)

func (m topologyBuilderMock) Build(_ mk8s.IgnoreWrapper) (*topology.Topology, error) {
	return m()
}

type tcpStateTableMock func(svcPort mk8s.ServiceWithPort) (int32, bool)

func (t tcpStateTableMock) Find(svcPort mk8s.ServiceWithPort) (int32, bool) {
	return t(svcPort)
}

func TestProvider(t *testing.T) {
	tests := []struct {
		desc               string
		acl                bool
		defaultTrafficType string
		tcpStateTable      map[mk8s.ServiceWithPort]int32
		topology           string
		wantConfig         string
	}{
		{
			desc:               "Annotations: traffic-type",
			acl:                false,
			defaultTrafficType: "http",
			tcpStateTable: map[mk8s.ServiceWithPort]int32{
				{Namespace: "my-ns", Name: "svc-a", Port: 8080}: 5000,
			},
			topology:   "fixtures/annotations-traffic-type-topology.json",
			wantConfig: "fixtures/annotations-traffic-type-config.json",
		},
		{
			desc:               "Annotations: scheme",
			acl:                false,
			defaultTrafficType: "http",
			topology:           "fixtures/annotations-scheme-topology.json",
			wantConfig:         "fixtures/annotations-scheme-config.json",
		},
		{
			desc:               "ACL disabled: basic HTTP service",
			acl:                false,
			defaultTrafficType: "http",
			topology:           "fixtures/acl-disabled-http-basic-topology.json",
			wantConfig:         "fixtures/acl-disabled-http-basic-config.json",
		},
		{
			desc:               "ACL disabled: basic TCP service",
			acl:                false,
			defaultTrafficType: "tcp",
			tcpStateTable: map[mk8s.ServiceWithPort]int32{
				{Namespace: "my-ns", Name: "svc-a", Port: 8080}: 5000,
			},
			topology:   "fixtures/acl-disabled-tcp-basic-topology.json",
			wantConfig: "fixtures/acl-disabled-tcp-basic-config.json",
		},
		{
			desc:               "ACL disabled: HTTP service with traffic-split",
			acl:                false,
			defaultTrafficType: "http",
			topology:           "fixtures/acl-disabled-http-traffic-split-topology.json",
			wantConfig:         "fixtures/acl-disabled-http-traffic-split-config.json",
		},
		{
			desc:               "ACL enabled: basic HTTP service",
			acl:                true,
			defaultTrafficType: "http",
			topology:           "fixtures/acl-enabled-http-basic-topology.json",
			wantConfig:         "fixtures/acl-enabled-http-basic-config.json",
		},
		{
			desc:               "ACL enabled: basic TCP service",
			acl:                true,
			defaultTrafficType: "tcp",
			tcpStateTable: map[mk8s.ServiceWithPort]int32{
				{Namespace: "my-ns", Name: "svc-b", Port: 8080}: 5000,
			},
			topology:   "fixtures/acl-enabled-tcp-basic-topology.json",
			wantConfig: "fixtures/acl-enabled-tcp-basic-config.json",
		},
		{
			desc:               "ACL enabled: HTTP service with traffic-split",
			acl:                true,
			defaultTrafficType: "http",
			topology:           "fixtures/acl-enabled-http-traffic-split-topology.json",
			wantConfig:         "fixtures/acl-enabled-http-traffic-split-config.json",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			logger := logrus.New()
			logger.SetOutput(ioutil.Discard)

			defaultTrafficType := "http"
			if test.defaultTrafficType != "" {
				defaultTrafficType = test.defaultTrafficType
			}

			cfg := provider.Config{
				IgnoredResources:   mk8s.NewIgnored(),
				MinHTTPPort:        10000,
				MaxHTTPPort:        10010,
				ACL:                test.acl,
				DefaultTrafficType: defaultTrafficType,
				MaeshNamespace:     "maesh",
			}
			builder := func() (*topology.Topology, error) {
				return loadTopology(test.topology)
			}
			tcpStateTable := func(port mk8s.ServiceWithPort) (int32, bool) {
				if test.tcpStateTable == nil {
					return 0, false
				}

				p, ok := test.tcpStateTable[port]
				return p, ok
			}
			p := provider.New(topologyBuilderMock(builder), tcpStateTableMock(tcpStateTable), cfg, event.NewLogrusReporter(logger))

			got, err := p.BuildConfig()
			require.NoError(t, err)

			assertConfig(t, test.wantConfig, got)
		})
	}
}

func loadTopology(filename string) (*topology.Topology, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var top topology.Topology

	if err = json.Unmarshal(data, &top); err != nil {
		return nil, err
	}

	return &top, nil
}

func assertConfig(t *testing.T, filename string, got *dynamic.Configuration) {
	data, err := ioutil.ReadFile(filename)
	require.NoError(t, err)

	var want dynamic.Configuration

	err = json.Unmarshal(data, &want)
	require.NoError(t, err)

	wantMarshaled, err := json.MarshalIndent(&want, "", "  ")
	require.NoError(t, err)

	gotMarshaled, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)

	assert.Equal(t, string(wantMarshaled), string(gotMarshaled))
}
