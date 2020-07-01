package controller

import (
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/sirupsen/logrus"
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
	table         map[int32]*servicePort
	logger        logrus.FieldLogger
}

// servicePort holds a combination of service namespace, name and port.
type servicePort struct {
	Namespace string
	Name      string
	Port      int32
}

// NewPortMapping creates and returns a new PortMapping instance.
func NewPortMapping(namespace string, serviceLister listers.ServiceLister, logger logrus.FieldLogger, minPort, maxPort int32) *PortMapping {
	return &PortMapping{
		namespace:     namespace,
		serviceLister: serviceLister,
		minPort:       minPort,
		maxPort:       maxPort,
		table:         make(map[int32]*servicePort),
		logger:        logger,
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
		namespace, name, err := p.parseServiceNamespaceAndName(shadowService.Name)
		if err != nil {
			p.logger.Error("Unable to parse shadow service shadowSvcName %q: %v", shadowService.Name, err)
			continue
		}

		for _, port := range shadowService.Spec.Ports {
			targetPort := port.TargetPort.IntVal

			if targetPort >= p.minPort && targetPort <= p.maxPort {
				p.table[targetPort] = &servicePort{
					Namespace: namespace,
					Name:      name,
					Port:      port.Port,
				}
			}
		}
	}

	return nil
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
	for i := p.minPort; i <= p.maxPort; i++ {
		// Skip until an available port is found
		if _, exists := p.table[i]; exists {
			continue
		}

		p.mu.Lock()
		p.table[i] = &servicePort{
			Namespace: namespace,
			Name:      name,
			Port:      port,
		}
		p.mu.Unlock()

		return i, nil
	}

	return 0, errors.New("unable to find an available port")
}

// Remove removes the mapping associated with the given service port.
func (p *PortMapping) Remove(namespace, name string, port int32) (int32, error) {
	port, ok := p.Find(namespace, name, port)
	if !ok {
		return 0, fmt.Errorf("unable to find port mapping for service %s/%s on port %d", namespace, name, port)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.table, port)

	return port, nil
}

// parseServiceNamespaceAndName parses and returns the service namespace and shadowSvcName from the given shadow service shadowSvcName.
func (p *PortMapping) parseServiceNamespaceAndName(shadowServiceName string) (namespace string, name string, err error) {
	expr := fmt.Sprintf(`%s-(.*)-6d61657368-(.*)`, p.namespace)

	regex, err := regexp.Compile(expr)
	if err != nil {
		return "", "", err
	}

	parts := regex.FindStringSubmatch(shadowServiceName)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("unable to parse service namespace and shadowSvcName")
	}

	return parts[2], parts[1], nil
}
