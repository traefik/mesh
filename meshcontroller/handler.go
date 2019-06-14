package meshcontroller

import (
	"fmt"

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
	Clients           *k8s.ClientWrapper
	IgnoredNamespaces k8s.Namespaces
}

func NewHandler(clients *k8s.ClientWrapper, namespaces k8s.Namespaces) *Handler {
	h := &Handler{
		Clients:           clients,
		IgnoredNamespaces: namespaces,
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
	service := obj.(*corev1.Service)
	if h.IgnoredNamespaces.Contains(service.Namespace) {
		return
	}

	log.Debugf("MeshControllerHandler ObjectCreated with type: *corev1.Service: %s/%s", service.Namespace, service.Name)

	log.Debugf("Verifying associated mesh service for service: %s/%s", service.Namespace, service.Name)
	if err := h.verifyMeshServiceExists(service); err != nil {
		log.Errorf("Could not verify mesh service exists: %v", err)
		return
	}

	log.Debugf("Verifying associated mesh ingressroute for service: %s/%s", service.Namespace, service.Name)
	if err := h.verifyMeshIngressRouteExists(service); err != nil {
		log.Errorf("Could not verify mesh ingressroute exists: %v", err)
	}

}

// ObjectDeleted is called when an object is deleted.
func (h *Handler) ObjectDeleted(obj interface{}) {
	// assert the type to an object to pull out relevant data
	service := obj.(*corev1.Service)

	if h.IgnoredNamespaces.Contains(service.Namespace) {
		return
	}
	log.Debugln("MeshControllerHandler.ObjectDeleted")
	if err := h.verifyMeshServiceDeleted(service); err != nil {
		log.Errorf("Could not verify mesh service deleted: %v", err)
		return
	}

	if err := h.verifyMeshIngressRouteDeleted(service); err != nil {
		log.Errorf("Could not verify mesh ingressroute deleted: %v", err)
	}
}

// ObjectUpdated is called when an object is updated.
func (h *Handler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debugln("MeshControllerHandler.ObjectUpdated")
}

func (h *Handler) verifyMeshServiceExists(service *apiv1.Service) error {
	meshServiceName := serviceToMeshName(service.Name, service.Namespace)
	meshServiceInstance, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if meshServiceInstance == nil || err != nil {
		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: k8s.MeshNamespace,
			},
			Spec: apiv1.ServiceSpec{
				Ports: []apiv1.ServicePort{
					{
						Name:       "web",
						Port:       80,
						TargetPort: intstr.FromInt(8000),
					},
				},
				Selector: map[string]string{
					"app": "traefik-mesh-node",
				},
			},
		}

		if _, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Create(svc); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) verifyMeshServiceDeleted(service *apiv1.Service) error {
	meshServiceName := serviceToMeshName(service.Name, service.Namespace)
	meshServiceInstance, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if meshServiceInstance != nil {
		// Service exists, delete
		if err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Delete(meshServiceName, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) verifyMeshIngressRouteExists(service *apiv1.Service) error {
	meshIngressRouteName := serviceToMeshName(service.Name, service.Namespace)
	matchRule := fmt.Sprintf("Host(`%s.%s.traefik.mesh`) || Host(`%s`)", service.Name, service.Namespace, service.Spec.ClusterIP)
	labels := map[string]string{
		"i3o-mesh": "internal",
	}

	meshIngressRouteInstance, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(service.Namespace).Get(meshIngressRouteName, metav1.GetOptions{})
	if meshIngressRouteInstance == nil || err != nil {
		ir := &traefikv1alpha1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshIngressRouteName,
				Namespace: service.Namespace,
				Labels:    labels,
			},
			Spec: traefikv1alpha1.IngressRouteSpec{
				Routes: []traefikv1alpha1.Route{
					{
						Match: matchRule,
						Kind:  "Rule",
						Services: []traefikv1alpha1.Service{
							{
								Name: service.Name,
								Port: service.Spec.Ports[0].Port,
							},
						},
					},
				},
			},
		}
		if _, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(metav1.NamespaceDefault).Create(ir); err != nil {
			return err
		}

	}

	return nil
}

func (h *Handler) verifyMeshIngressRouteDeleted(service *apiv1.Service) error {
	meshIngressRouteName := serviceToMeshName(service.Name, service.Namespace)
	meshIngressRouteInstance, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(service.Namespace).Get(meshIngressRouteName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if meshIngressRouteInstance != nil {
		// CRD exists, delete
		if err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(service.Namespace).Delete(meshIngressRouteName, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// serviceToMeshName converts a service with a namespace to a traefik-mesh ingressroute name.
func serviceToMeshName(serviceName string, namespace string) string {
	return fmt.Sprintf("traefik-%s-%s", namespace, serviceName)
}
