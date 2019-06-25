package mesh

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/containous/i3o/internal/controller/i3o"
	"github.com/containous/i3o/internal/k8s"
	traefikconfig "github.com/containous/traefik/pkg/config"
	traefikv1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
)

// MeshControllerHandler is an implementation of Handler.
type Handler struct {
	Clients    *k8s.ClientWrapper
	Ignored    k8s.IgnoreWrapper
	smiEnabled bool
}

func NewHandler(clients *k8s.ClientWrapper, ignored k8s.IgnoreWrapper, smiEnabled bool) *Handler {
	h := &Handler{
		Clients:    clients,
		Ignored:    ignored,
		smiEnabled: smiEnabled,
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
	switch obj := event.Object.(type) {

	case *corev1.Service:
		userService := obj
		if h.Ignored.Namespaces.Contains(userService.Namespace) {
			return
		}

		if h.Ignored.Services.Contains(userService.Name, userService.Namespace) {
			return
		}

		log.Debugf("MeshControllerHandler ObjectCreated with type: *corev1.Service: %s/%s", userService.Namespace, userService.Name)

		log.Debugf("Creating associated mesh service for service: %s/%s", userService.Namespace, userService.Name)
		createdService, err := h.createMeshService(userService)
		if err != nil {
			log.Errorf("Could not create mesh service: %v", err)
			return
		}

		if h.smiEnabled {
			if err := h.createBlockAllMiddleware(userService.Namespace); err != nil {
				log.Errorf("Could not create block-all middleware: %v", err)
				return
			}
		}

		if serviceType, ok := userService.Annotations[k8s.AnnotationServiceType]; ok {
			if strings.ToLower(serviceType) == k8s.ServiceTypeHTTP {
				// Use http ingressRoutes
				log.Debugf("Creating associated mesh ingressroute for service: %s/%s", userService.Namespace, userService.Name)
				if err := h.createMeshIngressRoutes(userService, createdService); err != nil {
					log.Errorf("Could not create mesh ingressroute: %v", err)
				}
				return
			}
		}

		// Default to use ingressRouteTCP
		log.Debugf("Creating associated mesh ingressrouteTCP for service: %s/%s", userService.Namespace, userService.Name)
		if err := h.createMeshIngressRouteTCPs(userService, createdService); err != nil {
			log.Errorf("Could not create mesh ingressrouteTCP: %v", err)
		}

	case *smiAccessv1alpha1.TrafficTarget:
		trafficTarget := obj

		if h.Ignored.Namespaces.Contains(trafficTarget.Namespace) {
			return
		}

		log.Debugf("MeshControllerHandler ObjectCreated with type: *smiAccessv1alpha1.TrafficTarget: %s/%s", trafficTarget.Namespace, trafficTarget.Name)

		if err := h.updateIngressRoutesWithSMI(trafficTarget); err != nil {
			log.Errorf("Could not update mesh services with smi: %v", err)
		}
	}

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
		log.Errorf("Could not verify mesh service deleted: %v", err)
		return
	}

	serviceType := userService.Annotations[k8s.AnnotationServiceType]
	if strings.ToLower(serviceType) == k8s.ServiceTypeHTTP {
		// Use http ingressRoutes
		log.Debugf("Deleting associated mesh ingressroute for service: %s/%s", userService.Namespace, userService.Name)
		if err := h.deleteMeshIngressRoutesByService(userService.Name, userService.Namespace); err != nil {
			log.Errorf("Could not delete mesh ingressroute: %v", err)
		}
		return
	}

	// Default to use ingressRouteTCP
	log.Debugf("Deleting associated mesh ingressrouteTCP for service: %s/%s", userService.Namespace, userService.Name)
	if err := h.deleteMeshIngressRouteTCPsByService(userService.Name, userService.Namespace); err != nil {
		log.Errorf("Could not delete mesh ingressroute: %v", err)
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

	updatedMeshService, err := h.updateMeshService(oldService, newService)
	if err != nil {
		log.Errorf("Could not update mesh service: %v", err)
		return
	}

	// Delete old routes based on old service.
	serviceType := oldService.Annotations[k8s.AnnotationServiceType]
	if strings.ToLower(serviceType) == k8s.ServiceTypeHTTP {
		// Use http ingressRoutes
		log.Debugf("Deleting associated mesh ingressroute for service: %s/%s", oldService.Namespace, oldService.Name)
		if err := h.deleteMeshIngressRoutesByService(oldService.Name, oldService.Namespace); err != nil {
			log.Errorf("Could not delete mesh ingressroute: %v", err)
		}
	} else {
		// Default to use ingressRouteTCP
		log.Debugf("Deleting associated mesh ingressrouteTCP for service: %s/%s", oldService.Namespace, oldService.Name)
		if err := h.deleteMeshIngressRouteTCPsByService(oldService.Name, oldService.Namespace); err != nil {
			log.Errorf("Could not delete mesh ingressroute: %v", err)
		}
	}

	// Create new routes based on new service.
	serviceType = newService.Annotations[k8s.AnnotationServiceType]
	if strings.ToLower(serviceType) == k8s.ServiceTypeHTTP {
		// Use http ingressRoutes
		log.Debugf("Creating associated mesh ingressroute for service: %s/%s", newService.Namespace, newService.Name)
		if err := h.createMeshIngressRoutes(newService, updatedMeshService); err != nil {
			log.Errorf("Could not crea mesh ingressroute: %v", err)
		}
	} else {
		// Default to use ingressRouteTCP
		log.Debugf("Creating associated mesh ingressrouteTCP for service: %s/%s", newService.Namespace, newService.Name)
		if err := h.createMeshIngressRouteTCPs(newService, updatedMeshService); err != nil {
			log.Errorf("Could not create mesh ingressrouteTCP: %v", err)
		}
	}
}

func (h *Handler) createMeshService(service *corev1.Service) (*corev1.Service, error) {
	meshServiceName := userServiceToMeshServiceName(service.Name, service.Namespace)
	meshServiceInstance, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if meshServiceInstance == nil || err != nil {
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

		return h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Create(svc)
	}

	return meshServiceInstance, nil
}

func (h *Handler) deleteMeshService(serviceName, serviceNamespace string) error {
	meshServiceName := userServiceToMeshServiceName(serviceName, serviceNamespace)
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

// updateMeshService updates the mesh service based on an old/new user service, and returns the updated mesh service
// for use to update the ingressRoutes[TCP]
func (h *Handler) updateMeshService(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error) {
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	meshServiceName := userServiceToMeshServiceName(oldUserService.Name, oldUserService.Namespace)

	var updatedSvc *corev1.Service
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		svc, err := h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if svc != nil {
			// Never modify the original object, always use a copy
			existing := svc.DeepCopy()
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

			existing.Spec.Ports = ports

			updatedSvc, err = h.Clients.KubeClient.CoreV1().Services(k8s.MeshNamespace).Update(existing)
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

func (h *Handler) createMeshIngressRoutes(userService *corev1.Service, createdService *corev1.Service) error {
	meshIngressRouteName := userServiceToMeshServiceName(userService.Name, userService.Namespace)
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

		if h.smiEnabled {
			ir.Spec.Routes[0].Middlewares = []traefikv1alpha1.MiddlewareRef{
				{
					Name:      "block-all-whitelist",
					Namespace: userService.Namespace,
				},
			}
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

func (h *Handler) createMeshIngressRouteTCPs(userService *corev1.Service, createdService *corev1.Service) error {
	meshIngressRouteName := userServiceToMeshServiceName(userService.Name, userService.Namespace)
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

func (h *Handler) deleteMeshIngressRoutesByService(serviceName, serviceNamespace string) error {
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

func (h *Handler) deleteMeshIngressRouteTCPsByService(serviceName, serviceNamespace string) error {
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

func (h *Handler) updateIngressRoutesWithSMI(obj interface{}) error {
	trafficTarget := obj.(*smiAccessv1alpha1.TrafficTarget)

	//	For each service, check to see if the destination SA applies,
	//	If it does, there needs to be a route for each source, to apply
	//	the source whitelisting to. This will eventually be extended to
	//	Add the HTTPRouteGroup filtering

	var sourceIPs []string
	for _, source := range trafficTarget.Sources {
		fieldSelector := fmt.Sprintf("spec.serviceAccountName=%s", source.Name)
		// Get all pods with the associated source serviceAccount (can only be in the source namespaces).
		pods, err := h.Clients.KubeClient.CoreV1().Pods(source.Namespace).List(metav1.ListOptions{FieldSelector: fieldSelector})
		if err != nil {
			return err
		}

		// Retrieve a list of sourceIPs from the list of pods.
		for _, pod := range pods.Items {
			sourceIPs = append(sourceIPs, pod.Status.PodIP)
		}
	}

	createdWhitelistMiddleware, err := h.createUpdateIPWhitelistMiddleware(trafficTarget.Name, trafficTarget.Destination.Namespace, sourceIPs)
	if err != nil {
		return err
	}

	// Get Endpoints in destination namespace.
	endpoints, err := h.Clients.KubeClient.CoreV1().Endpoints(trafficTarget.Destination.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints.Items {
		// Check Endpoints against service account via targetref in EndpointAddress.
		for _, subset := range endpoint.Subsets {
			var subsetMatch bool
			for _, endpointPort := range subset.Ports {
				if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port {
					subsetMatch = true
					break
				}
			}

			if !subsetMatch {
				// No subset port match on destination port, so subset is not affected
				continue
			}

			var validPodFound bool
			for _, address := range subset.Addresses {
				if pod, err := h.Clients.KubeClient.CoreV1().Pods(address.TargetRef.Namespace).Get(address.TargetRef.Name, metav1.GetOptions{}); err != nil {
					if err != nil {
						return err
					}
					if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
						validPodFound = true
						break
					}
				}
			}

			if !validPodFound {
				// No valid pods with serviceAccound found on the subset, so it is not affected
				continue
			}

			// We have a subset match, and valid referenced pods.
			// Update the ingressroute[tcp]s by adding the whitelist middleware

			for _, endpointPort := range subset.Ports {
				portString := strconv.FormatInt(int64(endpointPort.Port), 10)
				if portString != trafficTarget.Destination.Port {
					continue
				}
				meshIngressRouteName := userServiceToMeshServiceName(endpoint.Name, trafficTarget.Destination.Namespace) + "-" + portString

				retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					obj, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(trafficTarget.Destination.Namespace).Get(meshIngressRouteName, metav1.GetOptions{})
					if err != nil {
						return err
					}
					if obj != nil {
						// Never modify the original object, always use a copy
						ir := obj.DeepCopy()

						for x, route := range ir.Spec.Routes {
							blockAllKey := -1
							createdWhitelistKey := -1
							for i, mw := range route.Middlewares {
								if mw.Name == "block-all-whitelist" {
									blockAllKey = i
								}
								if mw.Name == createdWhitelistMiddleware.Name {
									createdWhitelistKey = i
								}
							}

							if blockAllKey == -1 {
								return fmt.Errorf("could not find block all middleware: %s/block-all-whitelist", trafficTarget.Destination.Namespace)
							}

							if createdWhitelistKey != -1 {
								// Middleware already exists in the route, no need to modify
								continue
							}

							//We have the key of the block-all middlware, so insert our middlware before
							middlewareList := append(route.Middlewares, traefikv1alpha1.MiddlewareRef{})
							copy(middlewareList[blockAllKey:], middlewareList[(blockAllKey-1):])
							middlewareList[blockAllKey] = traefikv1alpha1.MiddlewareRef{Name: createdWhitelistMiddleware.Name, Namespace: createdWhitelistMiddleware.Namespace}

							ir.Spec.Routes[x].Middlewares = middlewareList
						}

						_, err := h.Clients.CrdClient.TraefikV1alpha1().IngressRoutes(trafficTarget.Destination.Namespace).Update(ir)
						if err != nil {
							fmt.Println(err)
							return err
						}
					}
					return nil
				})

				if retryErr != nil {
					return fmt.Errorf("unable to update ingressRoute %q: %v", meshIngressRouteName, retryErr)
				}

			}
		}
	}

	return nil
}

func (h *Handler) createUpdateIPWhitelistMiddleware(name string, namespace string, ips []string) (*traefikv1alpha1.Middleware, error) {

	ipWhitelistMiddlewareName := name + namespace + "-whitelist"
	var createdMiddleware *traefikv1alpha1.Middleware
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		mw, err := h.Clients.CrdClient.TraefikV1alpha1().Middlewares(namespace).Get(ipWhitelistMiddlewareName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if mw != nil {
			// Never modify the original object, always use a copy
			// Update middleware, using a copy
			middleware := mw.DeepCopy()
			if middleware.Spec.IPWhiteList == nil {
				return fmt.Errorf("could not update mesh whitelist: %s/%s is not type whitelist", middleware.Namespace, middleware.Name)
			}

			middleware.Spec.IPWhiteList.SourceRange = ips

			createdMiddleware, err = h.Clients.CrdClient.TraefikV1alpha1().Middlewares(middleware.Namespace).Update(middleware)
			return err
		}

		// Create middleware.
		middleware := &traefikv1alpha1.Middleware{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ipWhitelistMiddlewareName,
				Namespace: namespace,
			},
			Spec: traefikconfig.Middleware{
				IPWhiteList: &traefikconfig.IPWhiteList{
					SourceRange: ips,
				},
			},
		}

		createdMiddleware, err = h.Clients.CrdClient.TraefikV1alpha1().Middlewares(middleware.Namespace).Create(middleware)
		return err
	})

	if retryErr != nil {
		return nil, fmt.Errorf("unable to update middleware %q: %v", ipWhitelistMiddlewareName, retryErr)
	}

	return createdMiddleware, nil
}

func (h *Handler) createBlockAllMiddleware(serviceNamespace string) error {
	_, err := h.Clients.CrdClient.TraefikV1alpha1().Middlewares(serviceNamespace).Get("block-all-whitelist", metav1.GetOptions{})
	if err == nil {
		//middleware already exists
		log.Debugf("Middleware: block-all-whitelist already exists in namespace: %s", serviceNamespace)
		return nil
	}

	middleware := &traefikv1alpha1.Middleware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "block-all-whitelist",
			Namespace: serviceNamespace,
		},
		Spec: traefikconfig.Middleware{
			IPWhiteList: &traefikconfig.IPWhiteList{
				SourceRange: []string{"255.255.255.255"},
			},
		},
	}

	log.Debugf("Creating middleware: block-all-whitelist in namespace: %s", serviceNamespace)
	_, err = h.Clients.CrdClient.TraefikV1alpha1().Middlewares(serviceNamespace).Create(middleware)
	return err
}

// userServiceToMeshServiceName converts a User service with a namespace to a traefik-mesh ingressroute name.
func userServiceToMeshServiceName(serviceName string, namespace string) string {
	return fmt.Sprintf("traefik-%s-%s", serviceName, namespace)
}
