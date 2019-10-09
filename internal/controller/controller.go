package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/providers/base"
	"github.com/containous/maesh/internal/providers/kubernetes"
	"github.com/containous/maesh/internal/providers/smi"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	smiAccessExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	smiSpecsExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	smiSplitExternalversions "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/util/retry"
)

// Controller hold controller configuration.
type Controller struct {
	clients           *k8s.ClientWrapper
	kubernetesFactory informers.SharedInformerFactory
	meshFactory       informers.SharedInformerFactory
	smiAccessFactory  smiAccessExternalversions.SharedInformerFactory
	smiSpecsFactory   smiSpecsExternalversions.SharedInformerFactory
	smiSplitFactory   smiSplitExternalversions.SharedInformerFactory
	handler           *Handler
	meshHandler       *Handler
	configRefreshChan chan bool
	provider          base.Provider
	ignored           k8s.IgnoreWrapper
	smiEnabled        bool
	defaultMode       string
	meshNamespace     string
	tcpStateTable     *k8s.State
	lastConfiguration safe.Safe
}

// NewMeshController is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper, smiEnabled bool, defaultMode string, meshNamespace string, ignoreNamespaces []string) *Controller {
	ignored := k8s.NewIgnored(meshNamespace, ignoreNamespaces)

	// configRefreshChan is used to trigger configuration refreshes and deploys.
	configRefreshChan := make(chan bool)

	handler := NewHandler(ignored, configRefreshChan)
	// Create a new mesh handler to handle mesh events (pods)
	meshHandler := NewHandler(ignored.WithoutMesh(), configRefreshChan)

	c := &Controller{
		clients:           clients,
		handler:           handler,
		meshHandler:       meshHandler,
		configRefreshChan: configRefreshChan,
		ignored:           ignored,
		smiEnabled:        smiEnabled,
		defaultMode:       defaultMode,
		meshNamespace:     meshNamespace,
	}

	if err := c.Init(); err != nil {
		log.Errorln("Could not initialize MeshController")
	}

	return c
}

// Init the Controller.
func (c *Controller) Init() error {
	// Register handler funcs to controller funcs.
	c.handler.RegisterMeshHandlers(c.createMeshService, c.updateMeshService, c.deleteMeshService)
	c.meshHandler.RegisterMeshHandlers(c.createMeshService, c.updateMeshService, c.deleteMeshService)

	// Create a new SharedInformerFactory, and register the event handler to informers.
	c.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.KubeClient, k8s.ResyncPeriod)
	c.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(c.handler)

	// Create a new SharedInformerFactory, and register the event handler to informers.
	c.meshFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.KubeClient,
		k8s.ResyncPeriod,
		informers.WithNamespace(c.meshNamespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = "component==maesh-mesh"
		}),
	)
	c.meshFactory.Core().V1().Pods().Informer().AddEventHandler(c.meshHandler)

	c.tcpStateTable = &k8s.State{Table: make(map[int]*k8s.ServiceWithPort)}

	if c.smiEnabled {
		c.provider = smi.New(c.clients, c.defaultMode, c.meshNamespace, c.ignored)

		// Create new SharedInformerFactories, and register the event handler to informers.
		c.smiAccessFactory = smiAccessExternalversions.NewSharedInformerFactoryWithOptions(c.clients.SmiAccessClient, k8s.ResyncPeriod)
		c.smiAccessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(c.handler)

		c.smiSpecsFactory = smiSpecsExternalversions.NewSharedInformerFactoryWithOptions(c.clients.SmiSpecsClient, k8s.ResyncPeriod)
		c.smiSpecsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(c.handler)

		c.smiSplitFactory = smiSplitExternalversions.NewSharedInformerFactoryWithOptions(c.clients.SmiSplitClient, k8s.ResyncPeriod)
		c.smiSplitFactory.Split().V1alpha1().TrafficSplits().Informer().AddEventHandler(c.handler)

		return nil
	}

	// If SMI is not configured, use the kubernetes provider.
	c.provider = kubernetes.New(c.clients, c.defaultMode, c.meshNamespace, c.tcpStateTable, c.ignored)

	return nil
}

// Run is the main entrypoint for the controller.
func (c *Controller) Run(stopCh <-chan struct{}) error {
	var err error
	// Handle a panic with logging and exiting.
	defer utilruntime.HandleCrash()

	log.Debug("Initializing Mesh controller")

	// Start the informers.
	c.startInformers(stopCh)

	// Load the state from the TCP State Configmap before running.
	c.tcpStateTable, err = c.loadTCPStateTable()
	if err != nil {
		log.Errorf("encountered error loading TCP state table: %v", err)
	}

	// Create the mesh services here to ensure that they exist
	log.Info("Creating initial mesh services")
	if err := c.createMeshServices(); err != nil {
		log.Errorf("could not create mesh services: %v", err)
	}

	for {
		select {
		case <-stopCh:
			log.Info("Shutting down workers")
			return nil
		case <-c.configRefreshChan:
			// Reload the configuration
			conf, err := c.provider.BuildConfig()
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(c.lastConfiguration.Get(), conf) {
				c.lastConfiguration.Set(conf)
				if deployErr := c.deployConfiguration(conf); deployErr != nil {
					return err
				}
			}
		}
	}
}

// startInformers starts the controller informers.
func (c *Controller) startInformers(stopCh <-chan struct{}) {
	// Start the informers
	c.kubernetesFactory.Start(stopCh)

	for t, ok := range c.kubernetesFactory.WaitForCacheSync(stopCh) {
		if !ok {
			log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	c.meshFactory.Start(stopCh)

	for t, ok := range c.meshFactory.WaitForCacheSync(stopCh) {
		if !ok {
			log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	if c.smiEnabled {
		c.smiAccessFactory.Start(stopCh)

		for t, ok := range c.smiAccessFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.smiSpecsFactory.Start(stopCh)

		for t, ok := range c.smiSpecsFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}

		c.smiSplitFactory.Start(stopCh)

		for t, ok := range c.smiSplitFactory.WaitForCacheSync(stopCh) {
			if !ok {
				log.Errorf("timed out waiting for controller caches to sync: %s", t.String())
			}
		}
	}
}

func (c *Controller) createMeshServices() error {
	services, err := c.clients.GetServices(metav1.NamespaceAll)
	if err != nil {
		return fmt.Errorf("unable to get services: %v", err)
	}

	for _, service := range services {
		if c.ignored.Ignored(service.Name, service.Namespace) {
			continue
		}

		log.Debugf("Creating mesh for service: %v", service.Name)
		meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)

		for _, subservice := range services {
			// If there is already a mesh service created, don't bother recreating
			if subservice.Name == meshServiceName && subservice.Namespace == c.meshNamespace {
				continue
			}
		}

		log.Infof("Creating associated mesh service: %s", meshServiceName)

		if err := c.createMeshService(service); err != nil {
			return fmt.Errorf("unable to get create mesh service: %v", err)
		}
	}

	return nil
}

func (c *Controller) createMeshService(service *corev1.Service) error {
	meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)
	log.Debugf("Creating mesh service: %s", meshServiceName)

	_, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
	if err != nil {
		return err
	}

	if !exists {
		// Mesh service does not exist.
		var ports []corev1.ServicePort

		serviceMode := service.Annotations[k8s.AnnotationServiceType]
		if serviceMode == "" {
			serviceMode = c.defaultMode
		}

		for id, sp := range service.Spec.Ports {
			if sp.Protocol != corev1.ProtocolTCP {
				log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, service.Namespace, service.Name)
				continue
			}

			targetPort := intstr.FromInt(5000 + id)
			if serviceMode == k8s.ServiceTypeTCP {
				targetPort = intstr.FromInt(c.getTCPPortFromState(service.Name, service.Namespace, sp.Port))
			}

			if targetPort.IntVal == 0 {
				log.Errorf("Could not get TCP Port for service: %s with service port: %v", service.Name, sp)
				continue
			}

			meshPort := corev1.ServicePort{
				Name:       sp.Name,
				Port:       sp.Port,
				TargetPort: targetPort,
			}

			ports = append(ports, meshPort)
		}

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: c.meshNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: ports,
				Selector: map[string]string{
					"component": "maesh-mesh",
				},
			},
		}

		_, err = c.clients.CreateService(svc)
	}

	return err
}

func (c *Controller) deleteMeshService(serviceName, serviceNamespace string) error {
	meshServiceName := c.userServiceToMeshServiceName(serviceName, serviceNamespace)

	_, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
	if err != nil {
		return err
	}

	if exists {
		// Service exists, delete
		if err := c.clients.DeleteService(c.meshNamespace, meshServiceName); err != nil {
			return err
		}

		log.Debugf("Deleted service: %s/%s", c.meshNamespace, meshServiceName)
	}

	return nil
}

// updateMeshService updates the mesh service based on an old/new user service, and returns the updated mesh service
func (c *Controller) updateMeshService(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error) {
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	meshServiceName := c.userServiceToMeshServiceName(oldUserService.Name, oldUserService.Namespace)

	var updatedSvc *corev1.Service

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, exists, err := c.clients.GetService(c.meshNamespace, meshServiceName)
		if err != nil {
			return err
		}

		if exists {
			var ports []corev1.ServicePort

			serviceMode := newUserService.Annotations[k8s.AnnotationServiceType]
			if serviceMode == "" {
				serviceMode = c.defaultMode
			}

			for id, sp := range newUserService.Spec.Ports {
				if sp.Protocol != corev1.ProtocolTCP {
					log.Warnf("Unsupported port type: %s, skipping port %s on service %s/%s", sp.Protocol, sp.Name, newUserService.Namespace, newUserService.Name)
					continue
				}

				targetPort := intstr.FromInt(5000 + id)
				if serviceMode == k8s.ServiceTypeTCP {
					targetPort = intstr.FromInt(c.getTCPPortFromState(newUserService.Name, newUserService.Namespace, sp.Port))
				}
				meshPort := corev1.ServicePort{
					Name:       sp.Name,
					Port:       sp.Port,
					TargetPort: targetPort,
				}

				ports = append(ports, meshPort)
			}

			newService := service.DeepCopy()
			newService.Spec.Ports = ports

			updatedSvc, err = c.clients.UpdateService(newService)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if retryErr != nil {
		return nil, fmt.Errorf("unable to update service %q: %v", meshServiceName, retryErr)
	}

	log.Debugf("Updated service: %s/%s", c.meshNamespace, meshServiceName)

	return updatedSvc, nil
}

// userServiceToMeshServiceName converts a User service with a namespace to a mesh service name.
func (c *Controller) userServiceToMeshServiceName(serviceName string, namespace string) string {
	return fmt.Sprintf("%s-%s-%s", c.meshNamespace, serviceName, namespace)
}

func (c *Controller) loadTCPStateTable() (*k8s.State, error) {
	result := c.tcpStateTable
	if result == nil {
		result = &k8s.State{Table: make(map[int]*k8s.ServiceWithPort)}
	}

	configMap, exists, err := c.clients.GetConfigMap(c.meshNamespace, k8s.TCPStateConfigMapName)
	if err != nil {
		return result, err
	}

	if !exists {
		return result, fmt.Errorf("TCP State Table configmap does not exist")
	}

	if len(configMap.Data) > 0 {
		for k, v := range configMap.Data {
			port, err := strconv.Atoi(k)
			if err != nil {
				continue
			}

			name, namespace, servicePort, err := k8s.ParseServiceNamePort(v)
			if err != nil {
				continue
			}

			result.Table[port] = &k8s.ServiceWithPort{
				Name:      name,
				Namespace: namespace,
				Port:      servicePort,
			}
		}
	}

	return result, nil
}

func (c *Controller) getTCPPortFromState(serviceName, serviceNamespace string, servicePort int32) int {
	for port, v := range c.tcpStateTable.Table {
		if v.Name == serviceName && v.Namespace == serviceNamespace && v.Port == servicePort {
			return port
		}
	}

	log.Debugf("No match found for %s/%s %d - Add a new port", serviceName, serviceNamespace, servicePort)
	// No Match, add new port
	for i := 10000; true; i++ {
		if _, exists := c.tcpStateTable.Table[i]; exists {
			// Port used
			continue
		}

		c.tcpStateTable.Table[i] = &k8s.ServiceWithPort{
			Name:      serviceName,
			Namespace: serviceNamespace,
			Port:      servicePort,
		}

		if err := c.saveTCPStateTable(); err != nil {
			log.Errorf("unable to save TCP state table config map: %v", err)
			return 0
		}

		return i
	}

	return 0
}

func (c *Controller) saveTCPStateTable() error {
	configMap, exists, err := c.clients.GetConfigMap(c.meshNamespace, k8s.TCPStateConfigMapName)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("TCP State Table configmap does not exist")
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newConfigMap := configMap.DeepCopy()

		if newConfigMap.Data == nil {
			newConfigMap.Data = make(map[string]string)
		}
		for k, v := range c.tcpStateTable.Table {
			key := strconv.Itoa(k)
			value := k8s.ServiceNamePortToString(v.Name, v.Namespace, v.Port)
			newConfigMap.Data[key] = value
		}
		_, err := c.clients.UpdateConfigMap(newConfigMap)
		return err
	})
}

// deployConfiguration deploys the configuration to the mesh pods.
func (c *Controller) deployConfiguration(config *dynamic.Configuration) error {
	podList, err := c.clients.ListPodWithOptions(c.meshNamespace, metav1.ListOptions{
		LabelSelector: "component==maesh-mesh",
	})
	if err != nil {
		return fmt.Errorf("unable to retrieve pod list: %v", err)
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("unable to find any active mesh pods to deploy config : %+v", config)
	}

	for _, pod := range podList.Items {
		log.Debugf("Deploying to pod %s with IP %s", pod.Name, pod.Status.PodIP)

		if deployErr := c.deployToPod(pod.Name, pod.Status.PodIP, config); deployErr != nil {
			log.Debugf("Error deploying configuration: %v", deployErr)
		}
	}

	return nil
}

func (c *Controller) deployToPod(name, ip string, config *dynamic.Configuration) error {
	if name == "" || ip == "" {
		// If there is no name or ip, then just return.
		return fmt.Errorf("pod has no name or IP")
	}

	b, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("unable to marshal configuration: %v", err)
	}

	url := fmt.Sprintf("http://%s:8080/api/providers/rest", ip)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("unable to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
		if _, bodyErr := ioutil.ReadAll(resp.Body); bodyErr != nil {
			return fmt.Errorf("unable to read response body: %v", bodyErr)
		}
	}
	if err != nil {
		return fmt.Errorf("unable to deploy configuration: %v", err)
	}

	return nil
}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "maesh-mesh"
}
