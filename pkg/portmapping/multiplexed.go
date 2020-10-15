package portmapping

import (
	"errors"
	"fmt"
	"sync"
)

type serviceNamespaceName struct {
	namespace string
	name      string
}

// MultiplexedPortMapping is a PortMapper that maps many service ports to a single target port.
type MultiplexedPortMapping struct {
	minPort int32
	maxPort int32
	mu      sync.RWMutex
	table   map[serviceNamespaceName]map[int32]int32
}

// NewMultiplexedPortMapping creates and returns a new MultiplexedPortMapping instance.
func NewMultiplexedPortMapping(minPort, maxPort int32) *MultiplexedPortMapping {
	return &MultiplexedPortMapping{
		minPort: minPort,
		maxPort: maxPort,
		table:   make(map[serviceNamespaceName]map[int32]int32),
	}
}

// Find searches the port mapped to the given service port.
func (m *MultiplexedPortMapping) Find(namespace, name string, port int32) (int32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mappings, found := m.table[serviceNamespaceName{namespace, name}]
	if !found {
		return 0, false
	}

	for targetPort, sourcePort := range mappings {
		if sourcePort == port {
			return targetPort, true
		}
	}

	return 0, false
}

// Add adds a new mapping between the given service port and the first port available in the range defined
// within minPort and maxPort. If there's no port left, an error will be returned.
func (m *MultiplexedPortMapping) Add(namespace, name string, port int32) (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	namespaceName := serviceNamespaceName{namespace, name}

	mapping, found := m.table[namespaceName]
	if !found {
		mapping = make(map[int32]int32)
		m.table[namespaceName] = mapping
	}

	// Check if the port is not already mapped.
	for targetPort, sourcePort := range mapping {
		if sourcePort == port {
			return targetPort, nil
		}
	}

	// Find a target port available and assign it.
	for i := m.minPort; i <= m.maxPort; i++ {
		if _, ok := mapping[i]; ok {
			continue
		}

		mapping[i] = port

		return i, nil
	}

	return 0, errors.New("unable to find an available port")
}

// Set sets the service port to the given target port. If given fromPort or toPort are already
// mapped, an error will be returned.
func (m *MultiplexedPortMapping) Set(namespace, name string, fromPort, toPort int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if toPort < m.minPort || toPort > m.maxPort {
		return fmt.Errorf("port must be between %d and %d, got %d", m.minPort, m.maxPort, toPort)
	}

	namespaceName := serviceNamespaceName{namespace, name}

	mapping, found := m.table[namespaceName]
	if !found {
		mapping = make(map[int32]int32)
		m.table[namespaceName] = mapping
	}

	for targetPort, sourcePort := range mapping {
		if targetPort == toPort || sourcePort == fromPort {
			return fmt.Errorf("port %d is already mapped to port %d", sourcePort, targetPort)
		}
	}

	mapping[toPort] = fromPort

	return nil
}

// Remove removes the mapping associated with the given service port.
func (m *MultiplexedPortMapping) Remove(namespace, name string, port int32) (int32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	namespaceName := serviceNamespaceName{namespace, name}

	mappings, found := m.table[namespaceName]
	if !found {
		return 0, false
	}

	for targetPort, sourcePort := range mappings {
		if sourcePort == port {
			delete(mappings, targetPort)

			if len(mappings) == 0 {
				delete(m.table, namespaceName)
			}

			return targetPort, true
		}
	}

	return 0, false
}
