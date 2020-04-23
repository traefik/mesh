package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPortMapping_GetEmptyState(t *testing.T) {
	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-state-table",
			Namespace: "maesh",
		},
	}
	client := fake.NewSimpleClientset(cfgMap)

	m, err := NewPortMapping(client, "maesh", "tcp-state-table", 10000, 10200)
	require.NoError(t, err)

	svc := m.Get(8080)

	assert.Nil(t, svc)
}

func TestPortMapping_GetWithState(t *testing.T) {
	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-state-table",
			Namespace: "maesh",
		},
		Data: map[string]string{
			"10000": "my-ns/my-app:9090",
			"10001": "my-ns/my-app2:9092",
		},
	}
	client := fake.NewSimpleClientset(cfgMap)

	m, err := NewPortMapping(client, "maesh", "tcp-state-table", 10000, 10200)
	require.NoError(t, err)

	svc := m.Get(10000)
	require.NotNil(t, svc)

	assert.Equal(t, "my-ns", svc.Namespace)
	assert.Equal(t, "my-app", svc.Name)
	assert.Equal(t, int32(9090), svc.Port)

	svc = m.Get(10001)
	require.NotNil(t, svc)

	assert.Equal(t, "my-ns", svc.Namespace)
	assert.Equal(t, "my-app2", svc.Name)
	assert.Equal(t, int32(9092), svc.Port)
}

func TestPortMapping_AddWithState(t *testing.T) {
	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-state-table",
			Namespace: "maesh",
		},
	}
	client := fake.NewSimpleClientset(cfgMap)

	m, err := NewPortMapping(client, "maesh", "tcp-state-table", 10000, 10200)
	require.NoError(t, err)

	wantSvc := &ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := m.Add(wantSvc)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	gotSvc := m.Get(10000)
	require.NotNil(t, gotSvc)
	assert.Equal(t, wantSvc, gotSvc)

	cfgMap, err = client.CoreV1().ConfigMaps("maesh").Get("tcp-state-table", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, cfgMap.Data, 1)
	assert.Equal(t, "my-ns/my-app:9090", cfgMap.Data["10000"])
}

func TestPortMapping_AddOverflow(t *testing.T) {
	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-state-table",
			Namespace: "maesh",
		},
	}
	client := fake.NewSimpleClientset(cfgMap)

	var m *PortMapping
	m, err := NewPortMapping(client, "maesh", "tcp-state-table", 10000, 10001)
	require.NoError(t, err)

	wantSvc := &ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}

	var port int32
	port, err = m.Add(wantSvc)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	port, err = m.Add(wantSvc)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	_, err = m.Add(wantSvc)
	assert.Error(t, err)

	gotSvc := m.Get(10000)
	require.NotNil(t, gotSvc)
	assert.Equal(t, wantSvc, gotSvc)

	gotSvc = m.Get(10001)
	require.NotNil(t, gotSvc)
	assert.Equal(t, wantSvc, gotSvc)

	gotSvc = m.Get(10002)
	assert.Nil(t, gotSvc)

	cfgMap, err = client.CoreV1().ConfigMaps("maesh").Get("tcp-state-table", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, cfgMap.Data, 2)
}

func TestPortMapping_FindWithState(t *testing.T) {
	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-state-table",
			Namespace: "maesh",
		},
		Data: map[string]string{
			"10000": "my-ns/my-app:9090",
			"10002": "my-ns/my-app2:9092",
		},
	}
	client := fake.NewSimpleClientset(cfgMap)

	m, err := NewPortMapping(client, "maesh", "tcp-state-table", 10000, 10200)
	require.NoError(t, err)

	svc := ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, ok := m.Find(svc)
	require.True(t, ok)
	assert.Equal(t, int32(10000), port)

	svc = ServiceWithPort{
		Namespace: "my-ns2",
		Name:      "my-app",
		Port:      9090,
	}
	_, ok = m.Find(svc)
	assert.False(t, ok)

	port, err = m.Add(&svc)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	port, ok = m.Find(svc)
	require.True(t, ok)
	assert.Equal(t, int32(10001), port)
}

func TestParseServiceNamePort(t *testing.T) {
	testCases := []struct {
		desc          string
		given         string
		wantName      string
		wantNamespace string
		wantPort      int32
		wantError     bool
	}{
		{
			desc:          "simple parse",
			given:         "foo/bar:80",
			wantName:      "bar",
			wantNamespace: "foo",
			wantPort:      80,
		},
		{
			desc:          "missing port",
			given:         "foo/bar",
			wantName:      "",
			wantNamespace: "",
			wantPort:      0,
			wantError:     true,
		},
		{
			desc:          "invalid port",
			given:         "foo/bar:%",
			wantName:      "",
			wantNamespace: "",
			wantPort:      0,
			wantError:     true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			svc, err := parseServiceNamePort(test.given)
			if test.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.wantName, svc.Name)
			assert.Equal(t, test.wantNamespace, svc.Namespace)
			assert.Equal(t, test.wantPort, svc.Port)
		})
	}
}

func TestFormatServiceNamePort(t *testing.T) {
	got := formatServiceNamePort("svc-name", "ns", 8080)
	assert.Equal(t, "ns/svc-name:8080", got)
}

func TestPortMapping_Remove(t *testing.T) {
	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-state-table",
			Namespace: "maesh",
		},
		Data: map[string]string{
			"10000": "my-ns/my-app:9090",
		},
	}
	client := fake.NewSimpleClientset(cfgMap)

	m, err := NewPortMapping(client, "maesh", "tcp-state-table", 10000, 10200)
	require.NoError(t, err)

	svc := ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := m.Remove(svc)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	_, err = m.Remove(svc)
	assert.Error(t, err)

	unknownSvc := ServiceWithPort{
		Namespace: "my-unknown-ns",
		Name:      "my-unknown-app",
		Port:      8088,
	}
	_, err = m.Remove(unknownSvc)
	assert.Error(t, err)
}
