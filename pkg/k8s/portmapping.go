package k8s

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// PortMapping is a PortMapper backed by a Kubernetes ConfigMap.
type PortMapping struct {
	mu    sync.RWMutex
	table map[int32]*ServiceWithPort

	minPort int32
	maxPort int32

	client          kubernetes.Interface
	cfgMapNamespace string
	cfgMapName      string
}

// NewPortMapping creates a new PortMapping instance.
func NewPortMapping(client kubernetes.Interface, cfgMapNamespace, cfgMapName string, minPort, maxPort int32) (*PortMapping, error) {
	m := &PortMapping{
		minPort:         minPort,
		maxPort:         maxPort,
		table:           make(map[int32]*ServiceWithPort),
		client:          client,
		cfgMapNamespace: cfgMapNamespace,
		cfgMapName:      cfgMapName,
	}

	if err := m.loadState(); err != nil {
		return nil, err
	}

	return m, nil
}

// Find searches for the port which is associated with the given ServiceWithPort.
func (m *PortMapping) Find(svc ServiceWithPort) (int32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for port, v := range m.table {
		if v.Name == svc.Name && v.Namespace == svc.Namespace && v.Port == svc.Port {
			return port, true
		}
	}

	return 0, false
}

// Get returns the ServiceWithPort associated to the given port.
func (m *PortMapping) Get(srcPort int32) *ServiceWithPort {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.table[srcPort]
}

// Add adds a new mapping between the given ServiceWithPort and the first port available in the range defined
// within minPort and maxPort. If there's no port left, an error will be returned.
func (m *PortMapping) Add(svc *ServiceWithPort) (int32, error) {
	for i := m.minPort; i < m.maxPort+1; i++ {
		// Skip until an available port is found
		if _, exists := m.table[i]; exists {
			continue
		}

		m.mu.Lock()
		m.table[i] = svc
		m.mu.Unlock()

		if err := m.saveState(); err != nil {
			// If the state can't be saved, we are going to have a mismatch between the local table and the ConfigMap.
			// By not undoing the assignment on the local table we allow the state to converge in future calls to Add,
			// making it more robust to temporary failure.
			return 0, fmt.Errorf("unable to save port mapping: %w", err)
		}

		return i, nil
	}

	return 0, errors.New("unable to find an available port")
}

// Remove removes the mapping associated with the given ServiceWithPort.
func (m *PortMapping) Remove(svc ServiceWithPort) (int32, error) {
	port, ok := m.Find(svc)
	if !ok {
		return 0, fmt.Errorf("unable to find port mapping for service %s/%s on port %d", svc.Namespace, svc.Name, svc.Port)
	}

	m.mu.Lock()
	delete(m.table, port)
	m.mu.Unlock()

	if err := m.saveState(); err != nil {
		return 0, fmt.Errorf("unable to save port mapping: %w", err)
	}

	return port, nil
}

func (m *PortMapping) loadState() error {
	cfg, err := m.client.CoreV1().ConfigMaps(m.cfgMapNamespace).Get(m.cfgMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to load state from ConfigMap %q in namespace %q: %w", m.cfgMapName, m.cfgMapNamespace, err)
	}

	if len(cfg.Data) > 0 {
		m.mu.Lock()
		defer m.mu.Unlock()

		for k, v := range cfg.Data {
			port, err := strconv.ParseInt(k, 10, 32)
			if err != nil {
				continue
			}

			svc, err := parseServiceNamePort(v)
			if err != nil {
				continue
			}

			m.table[int32(port)] = svc
		}
	}

	return nil
}

func (m *PortMapping) saveState() error {
	cfg, err := m.client.CoreV1().ConfigMaps(m.cfgMapNamespace).Get(m.cfgMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cpy := cfg.DeepCopy()

		if cpy.Data == nil {
			cpy.Data = make(map[string]string)
		}

		m.mu.RLock()
		defer m.mu.RUnlock()

		for k, v := range m.table {
			key := strconv.Itoa(int(k))
			value := formatServiceNamePort(v.Name, v.Namespace, v.Port)
			cpy.Data[key] = value
		}

		_, err := m.client.CoreV1().ConfigMaps(cfg.Namespace).Update(cpy)

		return err
	})
}

func parseServiceNamePort(value string) (*ServiceWithPort, error) {
	service := strings.Split(value, ":")
	if len(service) != 2 {
		return nil, fmt.Errorf("unable to parse service into name and port")
	}

	port64, err := strconv.ParseInt(service[1], 10, 32)
	if err != nil {
		return nil, err
	}

	substring := strings.Split(service[0], "/")

	if len(substring) != 2 {
		return nil, errors.New("unable to parse service into namespace and name")
	}

	return &ServiceWithPort{
		Name:      substring[1],
		Namespace: substring[0],
		Port:      int32(port64),
	}, nil
}

func formatServiceNamePort(name, namespace string, port int32) (value string) {
	return fmt.Sprintf("%s/%s:%d", namespace, name, port)
}
