package controller

import (
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/containous/maesh/pkg/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listers "k8s.io/client-go/listers/core/v1"
)

// PortMapping is a PortMapper.
type PortMapping struct {
	namespace     string
	serviceLister listers.ServiceLister
	minPort       int32
	maxPort       int32
	mu            sync.RWMutex
	table         map[int32]*k8s.ServicePort
}

// NewPortMapping creates and returns a new PortMapping instance.
func NewPortMapping(namespace string, serviceLister listers.ServiceLister, minPort, maxPort int32) *PortMapping {
	return &PortMapping{
		namespace:     namespace,
		serviceLister: serviceLister,
		minPort:       minPort,
		maxPort:       maxPort,
		table:         make(map[int32]*k8s.ServicePort),
	}
}

// LoadState initializes the mapping table from the current shadow service state.
func (p *PortMapping) LoadState() error {
	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "maesh", "type": "shadow"},
	}

	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return err
	}

	shadowServices, err := p.serviceLister.Services(p.namespace).List(selector)
	if err != nil {
		return fmt.Errorf("unable to list shadow services: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, shadowService := range shadowServices {
		for _, port := range shadowService.Spec.Ports {
			targetPort := port.TargetPort.IntVal

			if targetPort >= p.minPort && targetPort <= p.maxPort {
				namespace, name, err := p.parseServiceNamespaceAndName(shadowService.Name)
				if err != nil {
					return err
				}

				p.table[targetPort] = &k8s.ServicePort{
					Namespace: namespace,
					Name:      name,
					Port:      port.Port,
				}
			}
		}
	}

	return nil
}

// Find searches for the port which is associated with the given ServicePort.
func (p *PortMapping) Find(svc k8s.ServicePort) (int32, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for port, v := range p.table {
		if v.Name == svc.Name && v.Namespace == svc.Namespace && v.Port == svc.Port {
			return port, true
		}
	}

	return 0, false
}

// Add adds a new mapping between the given ServicePort and the first port available in the range defined
// within minPort and maxPort. If there's no port left, an error will be returned.
func (p *PortMapping) Add(svc *k8s.ServicePort) (int32, error) {
	for i := p.minPort; i < p.maxPort+1; i++ {
		// Skip until an available port is found
		if _, exists := p.table[i]; exists {
			continue
		}

		p.mu.Lock()
		p.table[i] = svc
		p.mu.Unlock()

		return i, nil
	}

	return 0, errors.New("unable to find an available port")
}

// Remove removes the mapping associated with the given ServicePort.
func (p *PortMapping) Remove(svc k8s.ServicePort) (int32, error) {
	port, ok := p.Find(svc)
	if !ok {
		return 0, fmt.Errorf("unable to find port mapping for service %s/%s on port %d", svc.Namespace, svc.Name, svc.Port)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.table, port)

	return port, nil
}

// parseServiceNamespaceAndName parses and returns the service namespace and name associated to the given shadow service name.
func (p *PortMapping) parseServiceNamespaceAndName(shadowServiceName string) (namespace string, name string, err error) {
	expr := fmt.Sprintf(`%s-(.*)-6d61657368-(.*)`, p.namespace)

	regex, err := regexp.Compile(expr)
	if err != nil {
		return "", "", err
	}

	parts := regex.FindStringSubmatch(shadowServiceName)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("unable to parse service namespace and name")
	}

	return parts[2], parts[1], nil
}
