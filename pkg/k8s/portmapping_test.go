package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortMapping_GetEmptyState(t *testing.T) {
	p := NewPortMapping("maesh", nil, 10000, 10200)

	svc := p.table[8080]
	assert.Nil(t, svc)
}

func TestPortMapping_GetWithState(t *testing.T) {
	p := NewPortMapping("maesh", nil, 10000, 10200)

	p.table[10000] = &ServiceWithPort{Namespace: "my-ns", Name: "my-app", Port: 9090}
	p.table[10001] = &ServiceWithPort{Namespace: "my-ns", Name: "my-app2", Port: 9092}

	svc := p.table[10000]
	require.NotNil(t, svc)
	assert.Equal(t, "my-ns", svc.Namespace)
	assert.Equal(t, "my-app", svc.Name)
	assert.Equal(t, int32(9090), svc.Port)

	svc = p.table[10001]
	require.NotNil(t, svc)
	assert.Equal(t, "my-ns", svc.Namespace)
	assert.Equal(t, "my-app2", svc.Name)
	assert.Equal(t, int32(9092), svc.Port)
}

func TestPortMapping_AddEmptyState(t *testing.T) {
	p := NewPortMapping("maesh", nil, 10000, 10200)

	wantSvc := &ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := p.Add(wantSvc)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	gotSvc := p.table[10000]
	require.NotNil(t, gotSvc)
	assert.Equal(t, wantSvc, gotSvc)
}

func TestPortMapping_AddOverflow(t *testing.T) {
	p := NewPortMapping("maesh", nil, 10000, 10001)

	wantSvc := &ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}

	port, err := p.Add(wantSvc)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	port, err = p.Add(wantSvc)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	_, err = p.Add(wantSvc)
	assert.Error(t, err)

	gotSvc := p.table[10000]
	require.NotNil(t, gotSvc)
	assert.Equal(t, wantSvc, gotSvc)

	gotSvc = p.table[10001]
	require.NotNil(t, gotSvc)
	assert.Equal(t, wantSvc, gotSvc)

	gotSvc = p.table[10002]
	assert.Nil(t, gotSvc)
}

func TestPortMapping_FindWithState(t *testing.T) {
	p := NewPortMapping("maesh", nil, 10000, 10200)

	p.table[10000] = &ServiceWithPort{Namespace: "my-ns", Name: "my-app", Port: 9090}
	p.table[10002] = &ServiceWithPort{Namespace: "my-ns", Name: "my-app2", Port: 9092}

	svc := ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, ok := p.Find(svc)
	require.True(t, ok)
	assert.Equal(t, int32(10000), port)

	svc = ServiceWithPort{
		Namespace: "my-ns2",
		Name:      "my-app",
		Port:      9090,
	}
	_, ok = p.Find(svc)
	assert.False(t, ok)

	port, err := p.Add(&svc)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	port, ok = p.Find(svc)
	require.True(t, ok)
	assert.Equal(t, int32(10001), port)
}

func TestPortMapping_Remove(t *testing.T) {
	p := NewPortMapping("maesh", nil, 10000, 10200)

	p.table[10000] = &ServiceWithPort{Namespace: "my-ns", Name: "my-app", Port: 9090}

	svc := ServiceWithPort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := p.Remove(svc)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	_, err = p.Remove(svc)
	assert.Error(t, err)

	unknownSvc := ServiceWithPort{
		Namespace: "my-unknown-ns",
		Name:      "my-unknown-app",
		Port:      8088,
	}
	_, err = p.Remove(unknownSvc)
	assert.Error(t, err)
}
