package meshcontroller

import (
	"github.com/containous/i3o/utils"
	crdclientset "github.com/containous/traefik/pkg/provider/kubernetes/crd/generated/clientset/versioned"
	traefik_v1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// MeshControllerHandler is an implementation of Handler
type Handler struct {
	Clients           *utils.ClientWrapper
	IgnoredNamespaces []string
}

func NewHandler(clients *utils.ClientWrapper, namespaces []string) *Handler {
	h := &Handler{
		Clients:           clients,
		IgnoredNamespaces: namespaces,
	}

	if err := h.Init(); err != nil {
		log.Errorln("Could not initialize MeshControllerHandler")
	}

	return h
}

// Init handles any handler initialization
func (h *Handler) Init() error {
	log.Debugln("MeshControllerHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (h *Handler) ObjectCreated(obj interface{}) {
	// assert the type to an object to pull out relevant data
	service := obj.(*corev1.Service)
	if utils.Contains(h.IgnoredNamespaces, service.Namespace) {
		return
	}
	log.Debugln("MeshControllerHandler ObjectCreated with type: *corev1.Service")
	if err := VerifyMeshServiceExists(h.Clients.KubeClient, service); err != nil {
		log.Errorf("Could not verify mesh service exists: %v", err)
		return
	}

	if err := VerifyMeshIngressRouteExists(h.Clients.CrdClient, service); err != nil {
		log.Errorf("Could not verify mesh ingressroute exists: %v", err)
	}

}

// ObjectDeleted is called when an object is deleted
func (h *Handler) ObjectDeleted(obj interface{}) {
	// assert the type to an object to pull out relevant data
	service := obj.(*corev1.Service)

	if utils.Contains(h.IgnoredNamespaces, service.Namespace) {
		return
	}
	log.Debugln("MeshControllerHandler.ObjectDeleted")
	if err := VerifyMeshServiceDeleted(h.Clients.KubeClient, service); err != nil {
		log.Errorf("Could not verify mesh service deleted: %v", err)
		return
	}

	if err := VerifyMeshIngressRouteDeleted(h.Clients.CrdClient, service); err != nil {
		log.Errorf("Could not verify mesh ingressroute deleted: %v", err)
	}
}

// ObjectUpdated is called when an object is updated
func (h *Handler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debugln("MeshControllerHandler.ObjectUpdated")
}

func VerifyMeshServiceExists(client kubernetes.Interface, service *apiv1.Service) error {
	meshServiceName := utils.ServiceToMeshName(service.Name, service.Namespace)
	meshServiceInstance, err := client.CoreV1().Services(utils.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if meshServiceInstance == nil || err != nil {
		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: utils.MeshNamespace,
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

		if _, err := client.CoreV1().Services(utils.MeshNamespace).Create(svc); err != nil {
			return err
		}
	}

	return nil
}

func VerifyMeshServiceDeleted(client kubernetes.Interface, service *apiv1.Service) error {
	meshServiceName := utils.ServiceToMeshName(service.Name, service.Namespace)
	meshServiceInstance, err := client.CoreV1().Services(utils.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if meshServiceInstance != nil {
		// Service exists, delete
		if err := client.CoreV1().Services(utils.MeshNamespace).Delete(meshServiceName, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func VerifyMeshIngressRouteExists(client crdclientset.Interface, service *apiv1.Service) error {
	meshIngressRouteName := utils.ServiceToMeshName(service.Name, service.Namespace)
	meshIngressRouteInstance, err := client.TraefikV1alpha1().IngressRoutes(service.Namespace).Get(meshIngressRouteName, metav1.GetOptions{})
	if meshIngressRouteInstance == nil || err != nil {
		ir := &traefik_v1alpha1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshIngressRouteName,
				Namespace: service.Namespace,
			},
			Spec: traefik_v1alpha1.IngressRouteSpec{
				Routes: []traefik_v1alpha1.Route{
					{
						Services: []traefik_v1alpha1.Service{
							{
								Name: service.Name,
								Port: service.Spec.Ports[0].Port,
							},
						},
					},
				},
			},
		}
		if _, err := client.TraefikV1alpha1().IngressRoutes(metav1.NamespaceAll).Create(ir); err != nil {
			return err
		}

	}

	return nil
}

func VerifyMeshIngressRouteDeleted(client crdclientset.Interface, service *apiv1.Service) error {
	meshIngressRouteName := utils.ServiceToMeshName(service.Name, service.Namespace)
	meshIngressRouteInstance, err := client.TraefikV1alpha1().IngressRoutes(service.Namespace).Get(meshIngressRouteName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if meshIngressRouteInstance != nil {
		// CRD exists, delete
		if err := client.TraefikV1alpha1().IngressRoutes(service.Namespace).Delete(meshIngressRouteName, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}
