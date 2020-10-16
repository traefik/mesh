package portmapping

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortMapping_AddWithEmptyState(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	wantSp := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := p.Add(wantSp.Namespace, wantSp.Name, wantSp.Port)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	gotSp := p.table[10000]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp, gotSp)
}

func TestPortMapping_AddWithState(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	p.table[10000] = &servicePort{Namespace: "my-ns", Name: "my-app-1", Port: 9090}

	wantSp := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app-2",
		Port:      9091,
	}
	port, err := p.Add(wantSp.Namespace, wantSp.Name, wantSp.Port)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	gotSp := p.table[10001]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp, gotSp)
}

func TestPortMapping_AddExistingServicePort(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	p.table[10000] = &servicePort{Namespace: "my-ns", Name: "my-app", Port: 9090}

	wantSp := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := p.Add(wantSp.Namespace, wantSp.Name, wantSp.Port)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	gotSp := p.table[10000]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp, gotSp)
}

func TestPortMapping_AddOverflow(t *testing.T) {
	p := NewPortMapping(10000, 10001)

	wantSp1 := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}

	port, err := p.Add(wantSp1.Namespace, wantSp1.Name, wantSp1.Port)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	wantSp2 := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9091,
	}

	port, err = p.Add(wantSp2.Namespace, wantSp2.Name, wantSp2.Port)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	wantSp3 := &servicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9092,
	}

	_, err = p.Add(wantSp3.Namespace, wantSp3.Name, wantSp3.Port)
	assert.Error(t, err)

	gotSp := p.table[10000]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp1, gotSp)

	gotSp = p.table[10001]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp2, gotSp)

	gotSp = p.table[10002]
	assert.Nil(t, gotSp)
}

func TestPortMapping_Find(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	p.table[10000] = &servicePort{Namespace: "my-ns", Name: "my-app", Port: 9090}
	p.table[10002] = &servicePort{Namespace: "my-ns", Name: "my-app2", Port: 9092}

	port, ok := p.Find("my-ns", "my-app", 9090)
	require.True(t, ok)
	assert.Equal(t, int32(10000), port)

	port, ok = p.Find("my-ns", "my-app2", 9092)
	require.True(t, ok)
	assert.Equal(t, int32(10002), port)

	_, ok = p.Find("my-ns2", "my-app", 9090)
	assert.False(t, ok)
}

func TestPortMapping_Remove(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	p.table[10000] = &servicePort{Namespace: "my-ns", Name: "my-app", Port: 9090}

	port, ok := p.Remove("my-ns", "my-app", 9090)
	assert.True(t, ok)
	assert.Equal(t, int32(10000), port)

	_, exists := p.table[10000]
	assert.False(t, exists)

	_, ok = p.Remove("my-ns", "my-app", 9090)
	assert.False(t, ok)

	_, ok = p.Remove("unknown-ns", "unknown-app", 8088)
	assert.False(t, ok)
}

func TestPortMapping_SetNewMapping(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	wantSp := &servicePort{Namespace: "my-ns", Name: "my-app", Port: 8080}

	err := p.Set(wantSp.Namespace, wantSp.Name, wantSp.Port, 10000)
	assert.NoError(t, err)

	gotSp, ok := p.table[10000]
	assert.True(t, ok)
	assert.Equal(t, wantSp, gotSp)
}

func TestPortMapping_SetOutOfRange(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	err := p.Set("my-ns", "my-app", 8080, 9999)
	assert.Error(t, err)

	_, ok := p.table[9999]
	assert.False(t, ok)

	err = p.Set("my-ns", "my-app", 8080, 10201)
	assert.Error(t, err)

	_, ok = p.table[10201]
	assert.False(t, ok)
}

func TestPortMapping_SetPortAlreadyMapped(t *testing.T) {
	p := NewPortMapping(10000, 10200)

	wantSp := &servicePort{Namespace: "my-ns", Name: "my-app", Port: 8080}

	p.table[10000] = wantSp

	err := p.Set("my-ns", "my-app2", 8081, 10000)
	assert.Error(t, err)

	gotSp, ok := p.table[10000]
	assert.True(t, ok)
	assert.Equal(t, wantSp, gotSp)

	err = p.Set("my-ns", "my-app", 8080, 10001)
	assert.Error(t, err)

	_, ok = p.table[10001]
	assert.False(t, ok)
}
