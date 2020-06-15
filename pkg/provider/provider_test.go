package provider

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stateTableMock func(ns, name string, port int32) (int32, bool)

func (t stateTableMock) Find(ns, name string, port int32) (int32, bool) {
	return t(ns, name, port)
}

type servicePort struct {
	Namespace string
	Name      string
	Port      int32
}

func TestProvider_BuildConfig(t *testing.T) {
	tests := []struct {
		desc               string
		acl                bool
		defaultTrafficType string
		tcpStateTable      map[servicePort]int32
		udpStateTable      map[servicePort]int32
		topology           string
		wantConfig         string
	}{
		{
			desc:               "Annotations: traffic-type",
			acl:                false,
			defaultTrafficType: "http",
			tcpStateTable: map[servicePort]int32{
				{Namespace: "my-ns", Name: "svc-a", Port: 8080}: 5000,
			},
			udpStateTable: map[servicePort]int32{
				{Namespace: "my-ns", Name: "svc-b", Port: 8080}: 15000,
			},
			topology:   "testdata/annotations-traffic-type-topology.json",
			wantConfig: "testdata/annotations-traffic-type-config.json",
		},
		{
			desc:               "Annotations: scheme",
			acl:                false,
			defaultTrafficType: "http",
			topology:           "testdata/annotations-scheme-topology.json",
			wantConfig:         "testdata/annotations-scheme-config.json",
		},
		{
			desc:               "ACL disabled: basic HTTP service",
			acl:                false,
			defaultTrafficType: "http",
			topology:           "testdata/acl-disabled-http-basic-topology.json",
			wantConfig:         "testdata/acl-disabled-http-basic-config.json",
		},
		{
			desc:               "ACL disabled: basic TCP service",
			acl:                false,
			defaultTrafficType: "tcp",
			tcpStateTable: map[servicePort]int32{
				{Namespace: "my-ns", Name: "svc-a", Port: 8080}: 5000,
			},
			topology:   "testdata/acl-disabled-tcp-basic-topology.json",
			wantConfig: "testdata/acl-disabled-tcp-basic-config.json",
		},
		{
			desc:               "ACL disabled: basic UDP service",
			acl:                false,
			defaultTrafficType: "udp",
			udpStateTable: map[servicePort]int32{
				{Namespace: "my-ns", Name: "svc-a", Port: 8080}: 15000,
			},
			topology:   "testdata/acl-disabled-udp-basic-topology.json",
			wantConfig: "testdata/acl-disabled-udp-basic-config.json",
		},
		{
			desc:               "ACL disabled: HTTP service with traffic-split",
			acl:                false,
			defaultTrafficType: "http",
			topology:           "testdata/acl-disabled-http-traffic-split-topology.json",
			wantConfig:         "testdata/acl-disabled-http-traffic-split-config.json",
		},
		{
			desc:               "ACL enabled: basic HTTP service",
			acl:                true,
			defaultTrafficType: "http",
			topology:           "testdata/acl-enabled-http-basic-topology.json",
			wantConfig:         "testdata/acl-enabled-http-basic-config.json",
		},
		{
			desc:               "ACL enabled: basic TCP service",
			acl:                true,
			defaultTrafficType: "tcp",
			tcpStateTable: map[servicePort]int32{
				{Namespace: "my-ns", Name: "svc-b", Port: 8080}: 5000,
			},
			topology:   "testdata/acl-enabled-tcp-basic-topology.json",
			wantConfig: "testdata/acl-enabled-tcp-basic-config.json",
		},
		{
			desc:               "ACL enabled: HTTP service with traffic-split",
			acl:                true,
			defaultTrafficType: "http",
			topology:           "testdata/acl-enabled-http-traffic-split-topology.json",
			wantConfig:         "testdata/acl-enabled-http-traffic-split-config.json",
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

			cfg := Config{
				MinHTTPPort:        10000,
				MaxHTTPPort:        10010,
				ACL:                test.acl,
				DefaultTrafficType: defaultTrafficType,
			}

			tcpStateTable := func(ns, name string, port int32) (int32, bool) {
				if test.tcpStateTable == nil {
					return 0, false
				}

				p, ok := test.tcpStateTable[servicePort{Namespace: ns, Name: name, Port: port}]
				return p, ok
			}
			udpStateTable := func(ns, name string, port int32) (int32, bool) {
				if test.udpStateTable == nil {
					return 0, false
				}

				p, ok := test.udpStateTable[servicePort{Namespace: ns, Name: name, Port: port}]
				return p, ok
			}
			middlewareBuilder := func(a map[string]string) (map[string]*dynamic.Middleware, error) {
				return nil, nil
			}

			p := New(stateTableMock(tcpStateTable), stateTableMock(udpStateTable), middlewareBuilder, cfg, logger)

			topo, err := loadTopology(test.topology)
			require.NoError(t, err)

			got := p.BuildConfig(topo)

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
