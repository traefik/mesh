package meshcontroller

import (
	"fmt"
	"strings"

	"github.com/containous/i3o/k8s"
	traefikv1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MeshControllerHandler is an implementation of Handler.
type Handler struct {
	Clients *k8s.ClientWrapper
	Ignored k8s.IgnoreWrapper
}

func NewHandler(clients *k8s.ClientWrapper, ignored k8s.IgnoreWrapper) *Handler {
	h := &Handler{
		Clients: clients,
		Ignored: ignored,
	}

	if err := h.Init(); err != nil {
		log.Errorln("Could not initialize MeshControllerHandler")
	}

	return h
}

// Init handles any handler initialization.
func (h *Handler) Init() error {
	log.Debugln("MeshControllerHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created.
func (h *Handler) ObjectCreated(obj interface{}) {
	// assert the type to an object to pull out relevant data
	userService := obj.(*corev1.Service)
	if h.Ignored.Namespaces.Contains(userService.Namespace) {
		return
	}

	if h.Ignored.Services.Contains(userService.Name, userService.Namespace) {
		return
	}

	log.Debugf("MeshControllerHandler ObjectCreated with type: *corev1.Service: %s/%s", userService.Namespace, userService.Name)

	log.Debugf("Verifying associated mesh service for service: %s/%s", userService.Namespace, userService.Name)
	createdService, err := h.verifyMeshServiceExists(userService)
	if err != nil {
		log.Errorf("Could not verify mesh service exists: %v", err)
		return
	}

	if serviceType, ok := userService.Annotations[k8s.ServiceType]; ok {
		if strings.ToLower(serviceType) == "http" {
			// Use http ingressRoutes
			log.Debugf("Verifying associated mesh ingressroute for service: %s/%s", userService.Namespace, userService.Name)
			if err := h.verifyMeshIngressRouteExists(userService, createdService); err != nil {
				log.Errorf("Could not verify mesh ingressroute exists: %v", err)
			}
			return
		}
	}

	// Default to use ingressRouteTCP
	log.Debugf("Verifying associated mesh ingressrouteTCP for service: %s/%s", userService.Namespace, userService.Name)
	if err := h.verifyMeshIngressRouteTCPExists(userService, createdService); err != nil {
		log.Errorf("Could not verify mesh ingressrouteTCP exists: %v", err)
	}

}

// ObjectDeleted is called when an object is deleted.
func (h *Handler) ObjectDeleted(key string, obj interface{}) {
	name, namespace := keyToNameAndNamespace(key)
	log.Debugf("MeshControllerHandler.ObjectDeleted: %s", key)

	// assert the type to find out what was deleted
	if _, ok := obj.(corev1.Service); ok {
		// This is a service, process as a deleted service.
		if h.Ignored.Namespaces.Contains(namespace) {
			return
		}

		if h.Ignored.Services.Contains(name, namespace) {
			return
		}

		if err := h.verifyMeshServiceDeleted(name, namespace); err != nil {
			log.Errorf("Could not verify mesh service deleted: %v", err)
			return
		}

		// Since we don't have annotations from the key, delete both HTTP and TCP routes for the service
		if err := h.verifyMeshIngressRouteDeleted(name, namespace); err != nil {
			log.Errorf("Could not verify mesh ingressroute deleted: %v", err)
		}

		if err := h.verifyMeshIngressRouteTCPDeleted(name, namespace); err != nil {
			log.Errorf("Could not verify mesh ingressroute deleted: %v", err)
		}
	}

}

// ObjectUpdated is called when an object is updated.
func (h *Handler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debugln("MeshControllerHandler.ObjectUpdated")
}

func (h *Handler) verifyMeshServiceExists(service *apiv1.Service) (*apiv1.Service, error) {
	meshServiceName := serviceToMeshName(service.Name, service.Namespace)
	meshServiceInstance, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if meshServiceInstance == nil || err != nil {
		// Mesh service does not exist.
		var ports []apiv1.ServicePort

		for id, sp := range service.Spec.Ports {
			if sp.Protocol != corev1.ProtocolTCP {
				log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, service.Namespace, service.Name)
				continue
			}

			meshPort := apiv1.ServicePort{
				Name:       sp.Name,
				Port:       sp.Port,
				TargetPort: intstr.FromInt(5000 + id),
			}

			ports = append(ports, meshPort)
		}

		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: k8s.MeshNamespace,
			},
			Spec: apiv1.ServiceSpec{
				Ports: ports,
				Selector: map[string]string{
					"component": "i3o-mesh",
				},
			},
		}
		return h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Create(svc)
	}
	return meshServiceInstance, nil
}

func (h *Handler) verifyMeshServiceDeleted(serviceName, serviceNamespace string) error {
	meshServiceName := serviceToMeshName(serviceName, serviceNamespace)
	meshServiceInstance, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if meshServiceInstance != nil {
		// Service exists, delete
		if err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Delete(meshServiceName, &metav1.DeleteOptions{}); err != nil {
			return err
		}
		log.Debugf("Deleted service: %s/%s", k8s.MeshNamespace, meshServiceName)
	}

	return nil
}

func (h *Handler) verifyMeshIngressRouteExists(userService *apiv1.Service, createdService *apiv1.Service) error {
	meshIngressRouteName := serviceToMeshName(userService.Name, userService.Namespace)
	matchRule := fmt.Sprintf("Host(`%s.%s.traefik.mesh`) || Host(`%s`)", userService.Name, userService.Namespace, userService.Spec.ClusterIP)
	labels := map[string]string{
		"i3o-mesh":     "internal",
		"user-service": userService.Name,
	}

	for _, sp := range createdService.Spec.Ports {
		ir := &traefikv1alpha1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", meshIngressRouteName, sp.TargetPort.IntVal),
				Namespace: userService.Namespace,
				Labels:    labels,
			},
			Spec: traefikv1alpha1.IngressRouteSpec{
				EntryPoints: []string{fmt.Sprintf("ingress-%d", sp.TargetPort.IntVal)},
				Routes: []traefikv1alpha1.Route{
					{
						Match: matchRule,
						Kind:  "Rule",
						Services: []traefikv1alpha1.Service{
							{
								Name: userService.Name,
								Port: sp.Port,
							},
						},
					},
				},
			},
		}

		irInstance, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(ir.Namespace).Get(ir.Name, metav1.GetOptions{})
		if irInstance == nil || err != nil {
			if _, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(ir.Namespace).Create(ir); err != nil {
				return err
			}
		}

	}

	return nil
}

func (h *Handler) verifyMeshIngressRouteTCPExists(userService *apiv1.Service, createdService *apiv1.Service) error {
	meshIngressRouteName := serviceToMeshName(userService.Name, userService.Namespace)
	matchRule := fmt.Sprintf("HostSNI(`%s.%s.traefik.mesh`) || HostSNI(`%s`)", userService.Name, userService.Namespace, userService.Spec.ClusterIP)
	labels := map[string]string{
		"i3o-mesh":     "internal",
		"user-service": userService.Name,
	}

	for _, sp := range createdService.Spec.Ports {
		irtcp := &traefikv1alpha1.IngressRouteTCP{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", meshIngressRouteName, sp.TargetPort.IntVal),
				Namespace: userService.Namespace,
				Labels:    labels,
			},
			Spec: traefikv1alpha1.IngressRouteTCPSpec{
				EntryPoints: []string{fmt.Sprintf("ingress-%d", sp.TargetPort.IntVal)},
				Routes: []traefikv1alpha1.RouteTCP{
					{
						Match: matchRule,
						Services: []traefikv1alpha1.ServiceTCP{
							{
								Name: userService.Name,
								Port: sp.Port,
							},
						},
					},
				},
			},
		}

		irtcpInstance, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRouteTCPs(irtcp.Namespace).Get(irtcp.Name, metav1.GetOptions{})
		if irtcpInstance == nil || err != nil {
			if _, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRouteTCPs(irtcp.Namespace).Create(irtcp); err != nil {
				return err
			}
		}

	}

	return nil
}

func (h *Handler) verifyMeshIngressRouteDeleted(serviceName, serviceNamespace string) error {
	selector := fmt.Sprintf("user-service=%s", serviceName)
	irs, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(serviceNamespace).List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	for _, ir := range irs.Items {
		if err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(ir.Namespace).Delete(ir.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
		log.Debugf("Deleted IngressRoute: %s/%s", ir.Namespace, ir.Name)
	}

	return nil
}

func (h *Handler) verifyMeshIngressRouteTCPDeleted(serviceName, serviceNamespace string) error {
	selector := fmt.Sprintf("user-service=%s", serviceName)
	irtcps, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRouteTCPs(serviceNamespace).List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	for _, irtcp := range irtcps.Items {
		if err := h.Clients.CrdClient.TraefikV1alpha1().IngressRouteTCPs(irtcp.Namespace).Delete(irtcp.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
		log.Debugf("Deleted IngressRouteTCP: %s/%s", irtcp.Namespace, irtcp.Name)
	}

	return nil
}

// serviceToMeshName converts a service with a namespace to a traefik-mesh ingressroute name.
func serviceToMeshName(serviceName string, namespace string) string {
	return fmt.Sprintf("traefik-%s-%s", namespace, serviceName)
}

// keyToNameAndNamespace splits a key to key and namespace strings
func keyToNameAndNamespace(key string) (name, namespace string) {
	splitKey := strings.Split(key, "/")
	if len(splitKey) == 1 {
		// No namespace in the key, return key in default namespace
		return key, metav1.NamespaceDefault
	}

	return splitKey[1], splitKey[0]
}
