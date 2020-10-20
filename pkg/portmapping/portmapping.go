package portmapping

import (
	"errors"
	"fmt"
	"sync"
)

// PortMapping is a PortMapper that map one service port to one target port.
type PortMapping struct {
	minPort int32
	maxPort int32
	mu      sync.RWMutex
	table   map[int32]*servicePort
}

// servicePort holds a combination of service namespace, name and port.
type servicePort struct {
	Namespace string
	Name      string
	Port      int32
}

// NewPortMapping creates and returns a new PortMapping instance.
func NewPortMapping(minPort, maxPort int32) *PortMapping {
	return &PortMapping{
		minPort: minPort,
		maxPort: maxPort,
		table:   make(map[int32]*servicePort),
	}
}

// Find searches the port mapped to the given service port.
func (p *PortMapping) Find(namespace, name string, port int32) (int32, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for mappedPort, v := range p.table {
		if v.Name == name && v.Namespace == namespace && v.Port == port {
			return mappedPort, true
		}
	}

	return 0, false
}

// Add adds a new mapping between the given service port and the first port available in the range defined
// within minPort and maxPort. If there's no port left, an error will be returned.
func (p *PortMapping) Add(namespace, name string, port int32) (int32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var availablePort int32

	for targetPort := p.minPort; targetPort <= p.maxPort; targetPort++ {
		sp, exists := p.table[targetPort]
		if !exists && availablePort == 0 {
			availablePort = targetPort
			continue
		}

		// If the port is already mapped return immediately the existing target port.
		if exists && (sp.Namespace == namespace && sp.Name == name && sp.Port == port) {
			return targetPort, nil
		}
	}

	if availablePort == 0 {
		return 0, errors.New("unable to find an available port")
	}

	p.table[availablePort] = &servicePort{
		Namespace: namespace,
		Name:      name,
		Port:      port,
	}

	return availablePort, nil
}

// Set maps the service port to the given target port.
func (p *PortMapping) Set(namespace, name string, fromPort, toPort int32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if toPort < p.minPort || toPort > p.maxPort {
		return fmt.Errorf("port must be between %d and %d, got %d", p.minPort, p.maxPort, toPort)
	}

	// Check if the port mapping is not already set.
	for targetPort, sp := range p.table {
		if targetPort == toPort || (sp.Namespace == namespace && sp.Name == name && sp.Port == fromPort) {
			return fmt.Errorf("port %d is already mapped to port %d", sp.Port, targetPort)
		}
	}

	p.table[toPort] = &servicePort{
		Namespace: namespace,
		Name:      name,
		Port:      fromPort,
	}

	return nil
}

// Remove removes the mapping associated with the given service port.
func (p *PortMapping) Remove(namespace, name string, port int32) (int32, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var targetPort int32

	// Check if there is a port mapped to the given port and service.
	for mappedPort, v := range p.table {
		if v.Name == name && v.Namespace == namespace && v.Port == port {
			targetPort = mappedPort
		}
	}

	if targetPort == 0 {
		return 0, false
	}

	delete(p.table, targetPort)

	return targetPort, true
}
