package controller

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/pkg/annotations"
	"github.com/traefik/mesh/v2/pkg/k8s"
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
	Set(namespace, name string, port, targetPort int32) error
	Remove(namespace, name string, port int32) (int32, bool)
}

// ShadowServiceManager manages shadow services.
type ShadowServiceManager struct {
	logger             logrus.FieldLogger
	serviceLister      listers.ServiceLister
	namespace          string
	httpStateTable     PortMapper
	tcpStateTable      PortMapper
	udpStateTable      PortMapper
	defaultTrafficType string
	kubeClient         kubernetes.Interface
}

// LoadPortMapping loads the port mapping of existing shadow services into the different port mappers.
func (s *ShadowServiceManager) LoadPortMapping() error {
	shadowSvcs, err := s.getShadowServices()
	if err != nil {
		return fmt.Errorf("unable to list shadow services: %w", err)
	}

	for _, shadowSvc := range shadowSvcs {
		// If the traffic-type annotation has been manually removed we can't load its ports.
		trafficType, err := annotations.GetTrafficType(shadowSvc.Annotations)
		if errors.Is(err, annotations.ErrNotFound) {
			s.logger.Errorf("Unable to find traffic-type on shadow service %q", shadowSvc.Name)
			continue
		}

		if err != nil {
			s.logger.Errorf("Unable to load port mapping of shadow service %q: %v", shadowSvc.Name, err)
			continue
		}

		s.loadShadowServicePorts(shadowSvc, trafficType)
	}

	return nil
}

// SyncService synchronizes the given service and its shadow service. If the shadow service doesn't exist it will be
// created. If it exists it will be updated and if the service doesn't exist, the shadow service will be removed.
func (s *ShadowServiceManager) SyncService(ctx context.Context, namespace, name string) error {
	s.logger.Debugf("Syncing service %q in namespace %q...", name, namespace)

	shadowSvcName, err := GetShadowServiceName(namespace, name)
	if err != nil {
		s.logger.Errorf("Unable to sync service %q in namespace %q: %v", name, namespace, err)
		return nil
	}

	svc, err := s.serviceLister.Services(namespace).Get(name)
	if kerrors.IsNotFound(err) {
		return s.deleteShadowService(ctx, namespace, name, shadowSvcName)
	}

	if err != nil {
		return err
	}

	return s.upsertShadowService(ctx, svc, shadowSvcName)
}

// deleteShadowService deletes the shadow service associated with the given user service.
func (s *ShadowServiceManager) deleteShadowService(ctx context.Context, namespace, name, shadowSvcName string) error {
	shadowSvc, err := s.serviceLister.Services(s.namespace).Get(shadowSvcName)
	if kerrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	s.logger.Debugf("Deleting shadow service %q...", shadowSvcName)

	trafficType, err := annotations.GetTrafficType(shadowSvc.Annotations)
	if errors.Is(err, annotations.ErrNotFound) {
		s.logger.Errorf("Unable to find traffic-type of the shadow service for service %q in namespace %q", name, namespace)
		return nil
	}

	if err != nil {
		s.logger.Errorf("Unable to delete shadow service for service %q in namespace %q: %v", name, namespace, err)
		return nil
	}

	for _, sp := range shadowSvc.Spec.Ports {
		if err = s.unmapPort(namespace, name, trafficType, sp.Port); err != nil {
			s.logger.Errorf("Unable to unmap port %d of service %q in namespace %q: %v", sp.Port, name, namespace, err)
		}
	}

	err = s.kubeClient.CoreV1().Services(s.namespace).Delete(ctx, shadowSvcName, metav1.DeleteOptions{})
	if kerrors.IsNotFound(err) {
		return nil
	}

	return err
}

// upsertShadowService updates or create the shadow service associated with the given user service.
func (s *ShadowServiceManager) upsertShadowService(ctx context.Context, svc *corev1.Service, shadowSvcName string) error {
	trafficType, err := annotations.GetTrafficType(svc.Annotations)
	if err != nil && !errors.Is(err, annotations.ErrNotFound) {
		return fmt.Errorf("unable to create or update shadow service for service %q in namespace %q: %w", svc.Name, svc.Namespace, err)
	}

	if errors.Is(err, annotations.ErrNotFound) {
		trafficType = s.defaultTrafficType
	}

	shadowSvc, err := s.serviceLister.Services(s.namespace).Get(shadowSvcName)
	if kerrors.IsNotFound(err) {
		return s.createShadowService(ctx, svc, shadowSvcName, trafficType)
	}

	if err != nil {
		return err
	}

	if shadowSvc.Labels[k8s.LabelServiceNamespace] != svc.Namespace || shadowSvc.Labels[k8s.LabelServiceName] != svc.Name {
		return fmt.Errorf("service labels in %q does not match service name %q and namespace %q", shadowSvcName, svc.Name, svc.Namespace)
	}

	return s.updateShadowService(ctx, svc, shadowSvc, trafficType)
}

// createShadowService creates the shadow service associated with the given user service.
func (s *ShadowServiceManager) createShadowService(ctx context.Context, svc *corev1.Service, shadowSvcName, trafficType string) error {
	s.logger.Debugf("Creating shadow service %q...", shadowSvcName)

	ports := s.getServicePorts(svc, trafficType)
	if len(ports) == 0 {
		ports = []corev1.ServicePort{buildUnresolvablePort()}
	}

	shadowSvcLabels := k8s.ShadowServiceLabels()
	shadowSvcLabels[k8s.LabelServiceNamespace] = svc.Namespace
	shadowSvcLabels[k8s.LabelServiceName] = svc.Name

	shadowSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        shadowSvcName,
			Namespace:   s.namespace,
			Labels:      shadowSvcLabels,
			Annotations: map[string]string{},
		},
		Spec: corev1.ServiceSpec{
			Selector: k8s.ProxyLabels(),
			Ports:    ports,
		},
	}

	annotations.SetTrafficType(trafficType, shadowSvc.Annotations)

	_, err := s.kubeClient.CoreV1().Services(s.namespace).Create(ctx, shadowSvc, metav1.CreateOptions{})

	return err
}

// updateShadowService updates the given shadow service based on the given user service.
func (s *ShadowServiceManager) updateShadowService(ctx context.Context, svc, shadowSvc *corev1.Service, trafficType string) error {
	s.logger.Debugf("Updating shadow service %q...", shadowSvc.Name)

	s.cleanupShadowServicePorts(svc, shadowSvc, trafficType)

	ports := s.getServicePorts(svc, trafficType)
	if len(ports) == 0 {
		ports = []corev1.ServicePort{buildUnresolvablePort()}
	}

	shadowSvc = shadowSvc.DeepCopy()
	shadowSvc.Spec.Ports = ports

	annotations.SetTrafficType(trafficType, shadowSvc.Annotations)

	_, err := s.kubeClient.CoreV1().Services(s.namespace).Update(ctx, shadowSvc, metav1.UpdateOptions{})

	return err
}

// getServicePorts returns the ports of the given user service, mapped with port opened on the proxy.
func (s *ShadowServiceManager) getServicePorts(svc *corev1.Service, trafficType string) []corev1.ServicePort {
	var ports []corev1.ServicePort

	for _, sp := range svc.Spec.Ports {
		if !isPortCompatible(trafficType, sp) {
			s.logger.Warnf("Unsupported port type %q on %q service %q in namespace %q, skipping port %d", sp.Protocol, trafficType, svc.Name, svc.Namespace, sp.Port)
			continue
		}

		targetPort, err := s.mapPort(svc.Name, svc.Namespace, trafficType, sp.Port)
		if err != nil {
			s.logger.Errorf("Unable to map port %d for %q service %q in namespace %q: %v", sp.Port, trafficType, svc.Name, svc.Namespace, err)
			continue
		}

		ports = append(ports, corev1.ServicePort{
			Name:       sp.Name,
			Port:       sp.Port,
			Protocol:   sp.Protocol,
			TargetPort: intstr.FromInt(int(targetPort)),
		})
	}

	return ports
}

// cleanupShadowServicePorts unmap ports that have changed since the last update of the service.
func (s *ShadowServiceManager) cleanupShadowServicePorts(svc, shadowSvc *corev1.Service, trafficType string) {
	oldTrafficType, err := annotations.GetTrafficType(shadowSvc.Annotations)
	if errors.Is(err, annotations.ErrNotFound) {
		s.logger.Errorf("Unable find traffic-type for shadow service %q", shadowSvc.Name)
		return
	}

	if err != nil {
		s.logger.Errorf("Unable to clean up ports for shadow service %q: %v", shadowSvc.Name, err)
		return
	}

	var oldPorts []corev1.ServicePort

	// Release ports which have changed since the last update. This operation has to be done before mapping new
	// ports as the number of target ports available is limited.
	if oldTrafficType != trafficType {
		// All ports have to be released if the traffic-type has changed.
		oldPorts = shadowSvc.Spec.Ports
	} else {
		oldPorts = getRemovedOrUpdatedPorts(shadowSvc.Spec.Ports, svc.Spec.Ports)
	}

	for _, sp := range oldPorts {
		if err := s.unmapPort(svc.Namespace, svc.Name, oldTrafficType, sp.Port); err != nil {
			s.logger.Errorf("Unable to unmap port %d of service %q in namespace %q: %v", sp.Port, svc.Name, svc.Namespace)
		}
	}
}

// mapPort maps the given port to a port on the proxy, if not already done.
func (s *ShadowServiceManager) setPort(name, namespace, trafficType string, port, mappedPort int32) error {
	var stateTable PortMapper

	switch trafficType {
	case annotations.ServiceTypeHTTP:
		stateTable = s.httpStateTable
	case annotations.ServiceTypeTCP:
		stateTable = s.tcpStateTable
	case annotations.ServiceTypeUDP:
		stateTable = s.udpStateTable
	default:
		return fmt.Errorf("unknown traffic type %q", trafficType)
	}

	if err := stateTable.Set(namespace, name, port, mappedPort); err != nil {
		return err
	}

	s.logger.Debugf("Port %d of service %q in namespace %q has been loaded and is mapped to port %d", port, name, namespace, mappedPort)

	return nil
}

// mapPort maps the given port to a port on the proxy, if not already done.
func (s *ShadowServiceManager) mapPort(name, namespace, trafficType string, port int32) (int32, error) {
	var stateTable PortMapper

	switch trafficType {
	case annotations.ServiceTypeHTTP:
		stateTable = s.httpStateTable
	case annotations.ServiceTypeTCP:
		stateTable = s.tcpStateTable
	case annotations.ServiceTypeUDP:
		stateTable = s.udpStateTable
	default:
		return 0, fmt.Errorf("unknown traffic type %q", trafficType)
	}

	mappedPort, err := stateTable.Add(namespace, name, port)
	if err != nil {
		return 0, err
	}

	s.logger.Debugf("Port %d of service %q in namespace %q has been mapped to port %d", port, name, namespace, mappedPort)

	return mappedPort, nil
}

// unmapPort releases the port on the proxy associated with the given port. This released port can then be
// remapped later on. Port releasing is delegated to the different port mappers, following the given traffic type.
func (s *ShadowServiceManager) unmapPort(namespace, name, trafficType string, port int32) error {
	var stateTable PortMapper

	switch trafficType {
	case annotations.ServiceTypeHTTP:
		stateTable = s.httpStateTable
	case annotations.ServiceTypeTCP:
		stateTable = s.tcpStateTable
	case annotations.ServiceTypeUDP:
		stateTable = s.udpStateTable
	default:
		return fmt.Errorf("unknown traffic type %q", trafficType)
	}

	if mappedPort, ok := stateTable.Remove(namespace, name, port); ok {
		s.logger.Debugf("Port %d of service %q in namespace %q has been unmapped to port %d", port, name, namespace, mappedPort)
	}

	return nil
}

// getRemovedOrUpdatedPorts returns the list of ports which have been removed or updated in the newPorts slice.
// New ports won't be returned.
func getRemovedOrUpdatedPorts(oldPorts, newPorts []corev1.ServicePort) []corev1.ServicePort {
	var ports []corev1.ServicePort

	for _, oldPort := range oldPorts {
		var found bool

		for _, newPort := range newPorts {
			if oldPort.Port == newPort.Port && oldPort.Protocol == newPort.Protocol {
				found = true

				break
			}
		}

		if !found {
			ports = append(ports, oldPort)
		}
	}

	return ports
}

// isPortCompatible checks if the given port is compatible with the given traffic type.
func isPortCompatible(trafficType string, sp corev1.ServicePort) bool {
	switch trafficType {
	case annotations.ServiceTypeUDP:
		return sp.Protocol == corev1.ProtocolUDP
	case annotations.ServiceTypeTCP, annotations.ServiceTypeHTTP:
		return sp.Protocol == corev1.ProtocolTCP
	default:
		return false
	}
}

// buildUnresolvablePort builds a service port with a fake port. This fake port can be used as a placeholder when a service
// doesn't have any compatible ports.
func buildUnresolvablePort() corev1.ServicePort {
	return corev1.ServicePort{
		Name:     "unresolvable-port",
		Protocol: corev1.ProtocolTCP,
		Port:     1666,
	}
}

// loadShadowServicePorts loads the port mapping of the given shadow service into the different port mappers.
func (s *ShadowServiceManager) loadShadowServicePorts(shadowSvc *corev1.Service, trafficType string) {
	namespace := shadowSvc.Labels[k8s.LabelServiceNamespace]
	name := shadowSvc.Labels[k8s.LabelServiceName]

	for _, sp := range shadowSvc.Spec.Ports {
		if !isPortCompatible(trafficType, sp) {
			s.logger.Warnf("Unsupported port type %q on %q service %q in namespace %q, skipping port %d", sp.Protocol, trafficType, shadowSvc.Name, shadowSvc.Namespace, sp.Port)
			continue
		}

		if err := s.setPort(name, namespace, trafficType, sp.Port, sp.TargetPort.IntVal); err != nil {
			s.logger.Errorf("Unable to load port %d for %q service %q in namespace %q: %v", sp.Port, trafficType, shadowSvc.Name, shadowSvc.Namespace, err)
			continue
		}
	}
}

// getUserServices returns all shadow services.
func (s *ShadowServiceManager) getShadowServices() ([]*corev1.Service, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: k8s.ShadowServiceLabels(),
	})
	if err != nil {
		return []*corev1.Service{}, err
	}

	shadowSvcs, err := s.serviceLister.Services(s.namespace).List(selector)
	if err != nil {
		return []*corev1.Service{}, err
	}

	return shadowSvcs, nil
}

// GetShadowServiceName returns the shadow service name corresponding to the given service namespace and name.
func GetShadowServiceName(namespace, name string) (string, error) {
	hash := fnv.New128a()

	_, err := hash.Write([]byte(namespace + name))
	if err != nil {
		return "", fmt.Errorf("unable to hash service namespace and name: %w", err)
	}

	return fmt.Sprintf("shadow-svc-%x", hash.Sum(nil)), nil
}
