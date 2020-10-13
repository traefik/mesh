package portmapping

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MultiplexedPortMappingAddWithEmptyState(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	wantSp := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := m.Add(wantSp.Namespace, wantSp.Name, wantSp.Port)
	assert.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	mapping, ok := m.table[serviceNamespaceName{namespace: wantSp.Namespace, name: wantSp.Name}]
	require.True(t, ok)

	assert.Equal(t, int32(9090), mapping[10000])
}

func Test_MultiplexedPortMappingAddWithState(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	m.table[serviceNamespaceName{namespace: "my-ns", name: "my-app"}] = map[int32]int32{
		10000: 9090,
	}

	port, err := m.Add("my-ns", "my-app", 9091)
	assert.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	port, err = m.Add("my-ns", "my-app2", 9090)
	assert.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{
		{namespace: "my-ns", name: "my-app"}: {
			10000: 9090,
			10001: 9091,
		},
		{namespace: "my-ns", name: "my-app2"}: {
			10000: 9090,
		},
	}, m.table)
}

func Test_MultiplexedPortMappingAddExistingServicePort(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	m.table[serviceNamespaceName{namespace: "my-ns", name: "my-app"}] = map[int32]int32{
		10000: 9090,
	}

	port, err := m.Add("my-ns", "my-app", 9090)
	assert.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{
		{namespace: "my-ns", name: "my-app"}: {
			10000: 9090,
		},
	}, m.table)
}

func Test_MultiplexedPortMappingAddOverflow(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10001)

	port, err := m.Add("my-ns", "my-app", 9090)
	assert.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	port, err = m.Add("my-ns", "my-app", 9091)
	assert.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	_, err = m.Add("my-ns", "my-app", 9092)
	assert.Error(t, err)

	port, err = m.Add("my-ns", "my-app2", 9090)
	assert.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{
		{namespace: "my-ns", name: "my-app"}: {
			10000: 9090,
			10001: 9091,
		},
		{namespace: "my-ns", name: "my-app2"}: {
			10000: 9090,
		},
	}, m.table)
}

func Test_MultiplexedPortMappingFind(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	m.table[serviceNamespaceName{namespace: "my-ns", name: "my-app"}] = map[int32]int32{
		10000: 9090,
	}

	port, ok := m.Find("my-ns", "my-app", 9090)
	assert.True(t, ok)
	assert.Equal(t, int32(10000), port)

	_, ok = m.Find("my-ns", "my-app", 9091)
	assert.False(t, ok)

	_, ok = m.Find("my-ns", "my-app2", 9090)
	assert.False(t, ok)
}

func Test_MultiplexedPortMappingRemove(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	m.table[serviceNamespaceName{namespace: "my-ns", name: "my-app"}] = map[int32]int32{
		10000: 9090,
	}

	m.table[serviceNamespaceName{namespace: "my-ns", name: "my-app2"}] = map[int32]int32{
		10000: 9090,
		10001: 9091,
	}

	port, ok := m.Remove("my-ns", "my-app", 9090)
	assert.True(t, ok)
	assert.Equal(t, int32(10000), port)

	port, ok = m.Remove("my-ns", "my-app2", 9090)
	assert.True(t, ok)
	assert.Equal(t, int32(10000), port)

	_, ok = m.Remove("my-ns", "my-app", 9090)
	assert.False(t, ok)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{
		{namespace: "my-ns", name: "my-app2"}: {
			10001: 9091,
		},
	}, m.table)
}

func Test_MultiplexedPortMappingSetNewMapping(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	err := m.Set("my-ns", "my-app", 9090, 10000)
	assert.NoError(t, err)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{
		{namespace: "my-ns", name: "my-app"}: {
			10000: 9090,
		},
	}, m.table)
}

func Test_MultiplexedPortMappingSetOutOfRange(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	err := m.Set("my-ns", "my-app", 9090, 9999)
	assert.Error(t, err)

	err = m.Set("my-ns", "my-app", 9090, 10201)
	assert.Error(t, err)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{}, m.table)
}

func Test_MultiplexedPortMappingSetPortAlreadyMapped(t *testing.T) {
	m := NewMultiplexedPortMapping(10000, 10200)

	m.table[serviceNamespaceName{namespace: "my-ns", name: "my-app"}] = map[int32]int32{
		10000: 9090,
	}

	err := m.Set("my-ns", "my-app", 9090, 10001)
	assert.Error(t, err)

	err = m.Set("my-ns", "my-app", 9091, 10000)
	assert.Error(t, err)

	assert.Equal(t, map[serviceNamespaceName]map[int32]int32{
		{namespace: "my-ns", name: "my-app"}: {
			10000: 9090,
		},
	}, m.table)
}
