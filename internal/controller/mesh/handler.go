package mesh

import (
	"fmt"

	"github.com/containous/i3o/internal/controller/i3o"
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/config"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
)

// MeshControllerHandler is an implementation of Handler.
type Handler struct {
	Client  *k8s.ClientWrapper
	Ignored k8s.IgnoreWrapper
	queue   workqueue.RateLimitingInterface
}

func NewHandler(client *k8s.ClientWrapper, ignored k8s.IgnoreWrapper, queue workqueue.RateLimitingInterface) *Handler {

	h := &Handler{
		Client:  client,
		Ignored: ignored,
		queue:   queue,
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
func (h *Handler) ObjectCreated(event i3o.Message) {
	// assert the type to an object to pull out relevant data
	userService := event.Object.(*corev1.Service)
	if h.Ignored.Namespaces.Contains(userService.Namespace) {
		return
	}

	if h.Ignored.Services.Contains(userService.Name, userService.Namespace) {
		return
	}

	log.Debugf("MeshControllerHandler ObjectCreated with type: *corev1.Service: %s/%s", userService.Namespace, userService.Name)

	log.Debugf("Creating associated mesh service for service: %s/%s", userService.Namespace, userService.Name)
	_, err := h.createMeshService(userService)
	if err != nil {
		log.Errorf("Could not create mesh service: %v", err)
		return
	}

	// Add the event to the message queue to trigger a configuration deployment
	log.Warnf("Added *corev1.Service: %s/%s to the message queue", userService.Namespace, userService.Name)
	h.queue.Add(event)
}

// ObjectDeleted is called when an object is deleted.
func (h *Handler) ObjectDeleted(event i3o.Message) {
	// assert the type to an object to pull out relevant data
	userService := event.Object.(*corev1.Service)
	if h.Ignored.Namespaces.Contains(userService.Namespace) {
		return
	}

	if h.Ignored.Services.Contains(userService.Name, userService.Namespace) {
		return
	}

	log.Debugf("MeshControllerHandler ObjectDeleted with type: *corev1.Service: %s/%s", userService.Namespace, userService.Name)

	if err := h.deleteMeshService(userService.Name, userService.Namespace); err != nil {
		log.Errorf("Could not delete mesh service: %v", err)
		return
	}
}

// ObjectUpdated is called when an object is updated.
func (h *Handler) ObjectUpdated(event i3o.Message) {
	// assert the type to an object to pull out relevant data
	newService := event.Object.(*corev1.Service)
	oldService := event.OldObject.(*corev1.Service)

	if h.Ignored.Namespaces.Contains(newService.Namespace) {
		return
	}

	if h.Ignored.Services.Contains(newService.Name, newService.Namespace) {
		return
	}

	log.Debugf("MeshControllerHandler ObjectUdated with type: *corev1.Service: %s/%s", newService.Namespace, newService.Name)

	_, err := h.updateMeshService(oldService, newService)
	if err != nil {
		log.Errorf("Could not update mesh service: %v", err)
		return
	}
}

func (h *Handler) createMeshService(service *corev1.Service) (*corev1.Service, error) {
	meshServiceName := userServiceToMeshServiceName(service.Name, service.Namespace)
	meshServiceInstance, exists, err := h.Client.GetService(k8s.MeshNamespace, meshServiceName)
	if err != nil {
		return nil, err
	}

	if !exists {
		// Mesh service does not exist.
		var ports []corev1.ServicePort

		for id, sp := range service.Spec.Ports {
			if sp.Protocol != corev1.ProtocolTCP {
				log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, service.Namespace, service.Name)
				continue
			}

			meshPort := corev1.ServicePort{
				Name:       sp.Name,
				Port:       sp.Port,
				TargetPort: intstr.FromInt(5000 + id),
			}

			ports = append(ports, meshPort)
		}

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: k8s.MeshNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: ports,
				Selector: map[string]string{
					"component": "i3o-mesh",
				},
			},
		}

		return h.Client.CreateService(svc)
	}

	return meshServiceInstance, nil
}

func (h *Handler) deleteMeshService(serviceName, serviceNamespace string) error {
	meshServiceName := userServiceToMeshServiceName(serviceName, serviceNamespace)
	_, exists, err := h.Client.GetService(k8s.MeshNamespace, meshServiceName)
	if err != nil {
		return err
	}

	if exists {
		// Service exists, delete
		if err := h.Client.DeleteService(k8s.MeshNamespace, meshServiceName); err != nil {
			return err
		}
		log.Debugf("Deleted service: %s/%s", k8s.MeshNamespace, meshServiceName)
	}

	return nil
}

// updateMeshService updates the mesh service based on an old/new user service, and returns the updated mesh service
// for use to update the ingressRoutes[TCP]
func (h *Handler) updateMeshService(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error) {
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	meshServiceName := userServiceToMeshServiceName(oldUserService.Name, oldUserService.Namespace)

	var updatedSvc *corev1.Service
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, exists, err := h.Client.GetService(k8s.MeshNamespace, meshServiceName)
		if err != nil {
			return err
		}

		if exists {
			var ports []corev1.ServicePort

			for id, sp := range newUserService.Spec.Ports {
				if sp.Protocol != corev1.ProtocolTCP {
					log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, newUserService.Namespace, newUserService.Name)
					continue
				}

				meshPort := corev1.ServicePort{
					Name:       sp.Name,
					Port:       sp.Port,
					TargetPort: intstr.FromInt(5000 + id),
				}

				ports = append(ports, meshPort)
			}

			newService := service.DeepCopy()
			newService.Spec.Ports = ports

			updatedSvc, err = h.Client.UpdateService(newService)
			if err != nil {
				fmt.Println(err)
				return err
			}
		}
		return nil
	})

	if retryErr != nil {
		return nil, fmt.Errorf("unable to update service %q: %v", meshServiceName, retryErr)
	}

	log.Debugf("Updated service: %s/%s", k8s.MeshNamespace, meshServiceName)
	return updatedSvc, nil

}

// userServiceToMeshServiceName converts a User service with a namespace to a traefik-mesh ingressroute name.
func userServiceToMeshServiceName(serviceName string, namespace string) string {
	return fmt.Sprintf("traefik-%s-%s", serviceName, namespace)
}

func mergeConfigurations(a *config.Configuration, b *config.Configuration) *config.Configuration {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	result := a

	for key, value := range b.HTTP.Middlewares {
		result.HTTP.Middlewares[key] = value
	}
	for key, value := range b.HTTP.Routers {
		result.HTTP.Routers[key] = value
	}
	for key, value := range b.HTTP.Services {
		result.HTTP.Services[key] = value
	}

	// FIXME: Add rest of values to merge
	return result
}
