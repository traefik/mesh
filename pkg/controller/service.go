package controller

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/containous/maesh/pkg/annotations"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
)

// PortMapper is capable of storing and retrieving a port mapping for a given service.
type PortMapper interface {
	Find(namespace, name string, port int32) (int32, bool)
	Add(namespace, name string, port int32) (int32, error)
	Remove(namespace, name string, port int32) (int32, error)
}

// ShadowServiceManager manages shadow services.
type ShadowServiceManager struct {
	logger             logrus.FieldLogger
	serviceLister      listers.ServiceLister
	namespace          string
	tcpStateTable      PortMapper
	udpStateTable      PortMapper
	defaultTrafficType string
	minHTTPPort        int32
	maxHTTPPort        int32
	kubeClient         kubernetes.Interface
}

// NewShadowServiceManager returns new shadow service manager.
func NewShadowServiceManager(logger logrus.FieldLogger, serviceLister listers.ServiceLister, namespace string, tcpStateTable, udpStateTable PortMapper, defaultTrafficType string, minHTTPPort, maxHTTPPort int32, kubeClient kubernetes.Interface) *ShadowServiceManager {
	return &ShadowServiceManager{
		logger:             logger,
		serviceLister:      serviceLister,
		namespace:          namespace,
		tcpStateTable:      tcpStateTable,
		udpStateTable:      udpStateTable,
		defaultTrafficType: defaultTrafficType,
		minHTTPPort:        minHTTPPort,
		maxHTTPPort:        maxHTTPPort,
		kubeClient:         kubeClient,
	}
}

// CreateOrUpdate creates or updates the shadow service corresponding to the given service.
func (s *ShadowServiceManager) CreateOrUpdate(svc *corev1.Service) (*corev1.Service, error) {
	shadowSvcName := s.getShadowServiceName(svc.Namespace, svc.Name)

	shadowSvc, err := s.serviceLister.Services(s.namespace).Get(shadowSvcName)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, fmt.Errorf("unable to get shadow service %q: %w", shadowSvcName, err)
	}

	// Removes the current mappings for the ports that are not present in the new service version.
	// Current shadow service ports are equal to the ports mapped for the previous service version.
	// This step is required to free up some ports before allocation.
	s.removeUnusedPortMappings(shadowSvc, svc)

	ports, err := s.getShadowServicePorts(svc)
	if err != nil {
		return nil, fmt.Errorf("unable to get shadow service ports for service %s/%s: %w", svc.Namespace, svc.Name, err)
	}

	newShadowSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shadowSvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app":  "maesh",
				"type": "shadow",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: ports,
			Selector: map[string]string{
				"component": "maesh-mesh",
			},
		},
	}

	// If the kubernetes server version is 1.17+, then use the topology key.
	if major, minor := parseKubernetesServerVersion(s.kubeClient); major == 1 && minor >= 17 {
		newShadowSvc.Spec.TopologyKeys = []string{
			"kubernetes.io/hostname",
			"topology.kubernetes.io/zone",
			"topology.kubernetes.io/region",
			"*",
		}
	}

	if shadowSvc == nil {
		return s.kubeClient.CoreV1().Services(s.namespace).Create(newShadowSvc)
	}

	// Ensure that we are not leaking some port mappings if the traffic type of the new service version has been updated.
	// If the traffic has been updated, some ports may be missing if they are not suitable, and some target port values may not match.
	s.cleanupPortMappings(svc.Namespace, svc.Name, shadowSvc, newShadowSvc)

	shadowSvc = shadowSvc.DeepCopy()
	shadowSvc.Spec.Ports = newShadowSvc.Spec.Ports

	return s.kubeClient.CoreV1().Services(s.namespace).Update(shadowSvc)
}

// Delete deletes the shadow service associated with the given service.
func (s *ShadowServiceManager) Delete(namespace, name string) error {
	shadowSvcName := s.getShadowServiceName(namespace, name)

	shadowSvc, err := s.serviceLister.Services(s.namespace).Get(shadowSvcName)
	if kerrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	// Removes all port mappings for the deleted service.
	// Current shadow service ports are equal to the deleted service ports.
	for _, svcPort := range shadowSvc.Spec.Ports {
		s.removeServicePortMapping(namespace, name, svcPort)
	}

	return s.kubeClient.CoreV1().Services(s.namespace).Delete(shadowSvcName, &metav1.DeleteOptions{})
}

func (s *ShadowServiceManager) cleanupPortMappings(namespace, name string, oldShadowSvc, newShadowSvc *corev1.Service) {
	for _, oldPort := range oldShadowSvc.Spec.Ports {
		if !needsCleanup(newShadowSvc.Spec.Ports, oldPort) {
			continue
		}

		s.removeServicePortMapping(namespace, name, oldPort)
	}
}

func (s *ShadowServiceManager) removeUnusedPortMappings(shadowSvc, svc *corev1.Service) {
	if svc == nil || shadowSvc == nil {
		return
	}

	for _, shadowSvcPort := range shadowSvc.Spec.Ports {
		if containsPort(svc.Spec.Ports, shadowSvcPort) {
			continue
		}

		s.removeServicePortMapping(svc.Namespace, svc.Name, shadowSvcPort)
	}
}

func (s *ShadowServiceManager) removeServicePortMapping(namespace, name string, svcPort corev1.ServicePort) {
	// Nothing to do here as there is no port table for HTTP ports.
	if svcPort.TargetPort.IntVal < s.maxHTTPPort {
		return
	}

	switch svcPort.Protocol {
	case corev1.ProtocolTCP:
		if _, err := s.tcpStateTable.Remove(namespace, name, svcPort.Port); err != nil {
			s.logger.Warnf("Unable to remove TCP port mapping for %s/%s on port %d", namespace, name, svcPort.Port)
		}

	case corev1.ProtocolUDP:
		if _, err := s.udpStateTable.Remove(namespace, name, svcPort.Port); err != nil {
			s.logger.Warnf("Unable to remove UDP port mapping for %s/%s on port %d", namespace, name, svcPort.Port)
		}
	}
}

// getShadowServiceName returns the shadow service shadowSvcName corresponding to the given service shadowSvcName and namespace.
func (s *ShadowServiceManager) getShadowServiceName(namespace, name string) string {
	return fmt.Sprintf("%s-%s-6d61657368-%s", s.namespace, name, namespace)
}

func (s *ShadowServiceManager) getShadowServicePorts(svc *corev1.Service) ([]corev1.ServicePort, error) {
	var ports []corev1.ServicePort

	trafficType, err := annotations.GetTrafficType(s.defaultTrafficType, svc.Annotations)
	if err != nil {
		return nil, fmt.Errorf("unable to get service traffic-type: %w", err)
	}

	for i, sp := range svc.Spec.Ports {
		if !isPortSuitable(trafficType, sp) {
			s.logger.Warnf("Unsupported port type %q on %q service %s/%s, skipping port %q", sp.Protocol, trafficType, svc.Namespace, svc.Name, sp.Name)
			continue
		}

		targetPort, err := s.getTargetPort(trafficType, i, svc.Name, svc.Namespace, sp.Port)
		if err != nil {
			s.logger.Errorf("Unable to find available %s port: %v, skipping port %s on service %s/%s", sp.Name, err, sp.Name, svc.Namespace, svc.Name)
			continue
		}

		ports = append(ports, corev1.ServicePort{
			Name:       sp.Name,
			Port:       sp.Port,
			Protocol:   sp.Protocol,
			TargetPort: intstr.FromInt(int(targetPort)),
		})
	}

	return ports, nil
}

func (s *ShadowServiceManager) getTargetPort(trafficType string, portID int, name, namespace string, port int32) (int32, error) {
	switch trafficType {
	case annotations.ServiceTypeHTTP:
		return s.getHTTPPort(portID)
	case annotations.ServiceTypeTCP:
		return s.getMappedPort(s.tcpStateTable, name, namespace, port)
	case annotations.ServiceTypeUDP:
		return s.getMappedPort(s.udpStateTable, name, namespace, port)
	default:
		return 0, errors.New("unknown service mode")
	}
}

// getHTTPPort returns the HTTP port associated with the given portID.
func (s *ShadowServiceManager) getHTTPPort(portID int) (int32, error) {
	if s.minHTTPPort+int32(portID) >= s.maxHTTPPort {
		return 0, errors.New("unable to find an available HTTP port")
	}

	return s.minHTTPPort + int32(portID), nil
}

// getMappedPort returns the port associated with the given service information in the given port mapper.
func (s *ShadowServiceManager) getMappedPort(stateTable PortMapper, name, namespace string, port int32) (int32, error) {
	if mappedPort, ok := stateTable.Find(namespace, name, port); ok {
		return mappedPort, nil
	}

	s.logger.Debugf("No match found for %s/%s %d - Add a new port", namespace, name, port)

	mappedPort, err := stateTable.Add(namespace, name, port)
	if err != nil {
		return 0, fmt.Errorf("unable to add service to the TCP state table: %w", err)
	}

	s.logger.Debugf("Service %s/%s %d as been assigned port %d", namespace, name, port, mappedPort)

	return mappedPort, nil
}

func isPortSuitable(trafficType string, sp corev1.ServicePort) bool {
	if trafficType == annotations.ServiceTypeUDP {
		return sp.Protocol == corev1.ProtocolUDP
	}

	if trafficType == annotations.ServiceTypeTCP || trafficType == annotations.ServiceTypeHTTP {
		return sp.Protocol == corev1.ProtocolTCP
	}

	return false
}

func parseKubernetesServerVersion(kubeClient kubernetes.Interface) (major, minor int) {
	kubeVersion, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return 0, 0
	}

	major, err = strconv.Atoi(kubeVersion.Major)
	if err != nil {
		return 0, 0
	}

	minor, err = strconv.Atoi(kubeVersion.Minor)
	if err != nil {
		return 0, 0
	}

	return major, minor
}

// containsPort returns true if a service port with the same port and protocol value exist in the given port list, false otherwise.
func containsPort(ports []corev1.ServicePort, port corev1.ServicePort) bool {
	for _, onePort := range ports {
		if onePort.Port == port.Port && onePort.Protocol == port.Protocol {
			return true
		}
	}

	return false
}

// needsCleanup returns true if the given shadow service port have to be cleaned up, false otherwise.
func needsCleanup(ports []corev1.ServicePort, port corev1.ServicePort) bool {
	for _, onePort := range ports {
		if onePort.Port == port.Port && onePort.Protocol == port.Protocol && onePort.TargetPort == port.TargetPort {
			return false
		}
	}

	return true
}
