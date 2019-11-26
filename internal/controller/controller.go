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

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/providers/base"
	"github.com/containous/maesh/internal/providers/kubernetes"
	"github.com/containous/maesh/internal/providers/smi"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	accessInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	accessLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/listers/access/v1alpha1"
	specsInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	specsLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/listers/specs/v1alpha1"
	splitInformer "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	splitLister "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/listers/split/v1alpha2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/util/retry"
)

// Controller hold controller configuration.
type Controller struct {
	clients              *k8s.ClientWrapper
	kubernetesFactory    informers.SharedInformerFactory
	smiAccessFactory     accessInformer.SharedInformerFactory
	smiSpecsFactory      specsInformer.SharedInformerFactory
	smiSplitFactory      splitInformer.SharedInformerFactory
	handler              *Handler
	configRefreshChan    chan string
	provider             base.Provider
	ignored              k8s.IgnoreWrapper
	smiEnabled           bool
	defaultMode          string
	meshNamespace        string
	tcpStateTable        *k8s.State
	lastConfiguration    safe.Safe
	api                  *API
	apiPort              int
	deployLog            *DeployLog
	PodLister            listers.PodLister
	ConfigMapLister      listers.ConfigMapLister
	ServiceLister        listers.ServiceLister
	EndpointsLister      listers.EndpointsLister
	TrafficTargetLister  accessLister.TrafficTargetLister
	HTTPRouteGroupLister specsLister.HTTPRouteGroupLister
	TCPRouteLister       specsLister.TCPRouteLister
	TrafficSplitLister   splitLister.TrafficSplitLister
}

// NewMeshController is used to build the informers and other required components of the mesh controller,
// and return an initialized mesh controller object.
func NewMeshController(clients *k8s.ClientWrapper, smiEnabled bool, defaultMode string, meshNamespace string, ignoreNamespaces []string, apiPort int) *Controller {
	ignored := k8s.NewIgnored()

	for _, ns := range ignoreNamespaces {
		ignored.AddIgnoredNamespace(ns)
	}

	ignored.AddIgnoredService("kubernetes", metav1.NamespaceDefault)
	ignored.AddIgnoredNamespace(metav1.NamespaceSystem)
	ignored.AddIgnoredApps("maesh", "jaeger")

	// configRefreshChan is used to trigger configuration refreshes and deploys.
	configRefreshChan := make(chan string)
	handler := NewHandler(ignored, configRefreshChan)

	c := &Controller{
		clients:           clients,
		handler:           handler,
		configRefreshChan: configRefreshChan,
		ignored:           ignored,
		smiEnabled:        smiEnabled,
		defaultMode:       defaultMode,
		meshNamespace:     meshNamespace,
		apiPort:           apiPort,
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

	// Create a new SharedInformerFactory, and register the event handler to informers.
	c.kubernetesFactory = informers.NewSharedInformerFactoryWithOptions(c.clients.KubeClient, k8s.ResyncPeriod)
	c.kubernetesFactory.Core().V1().Services().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Endpoints().Informer().AddEventHandler(c.handler)
	c.kubernetesFactory.Core().V1().Pods().Informer().AddEventHandler(c.handler)

	c.tcpStateTable = &k8s.State{Table: make(map[int]*k8s.ServiceWithPort)}

	// Create the base listers
	c.PodLister = c.kubernetesFactory.Core().V1().Pods().Lister()
	c.ConfigMapLister = c.kubernetesFactory.Core().V1().ConfigMaps().Lister()
	c.ServiceLister = c.kubernetesFactory.Core().V1().Services().Lister()
	c.EndpointsLister = c.kubernetesFactory.Core().V1().Endpoints().Lister()

	c.deployLog = NewDeployLog(1000)
	c.api = NewAPI(c.apiPort, &c.lastConfiguration, c.deployLog, c.PodLister, c.meshNamespace)

	if c.smiEnabled {
		// Create new SharedInformerFactories, and register the event handler to informers.
		c.smiAccessFactory = accessInformer.NewSharedInformerFactoryWithOptions(c.clients.SmiAccessClient, k8s.ResyncPeriod)
		c.smiAccessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(c.handler)

		c.smiSpecsFactory = specsInformer.NewSharedInformerFactoryWithOptions(c.clients.SmiSpecsClient, k8s.ResyncPeriod)
		c.smiSpecsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(c.handler)
		c.smiSpecsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(c.handler)

		c.smiSplitFactory = splitInformer.NewSharedInformerFactoryWithOptions(c.clients.SmiSplitClient, k8s.ResyncPeriod)
		c.smiSplitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(c.handler)

		// Create the SMI listers
		c.TrafficTargetLister = c.smiAccessFactory.Access().V1alpha1().TrafficTargets().Lister()
		c.HTTPRouteGroupLister = c.smiSpecsFactory.Specs().V1alpha1().HTTPRouteGroups().Lister()
		c.TCPRouteLister = c.smiSpecsFactory.Specs().V1alpha1().TCPRoutes().Lister()
		c.TrafficSplitLister = c.smiSplitFactory.Split().V1alpha2().TrafficSplits().Lister()

		c.provider = smi.New(c.defaultMode, c.tcpStateTable, c.ignored, c.ServiceLister, c.EndpointsLister, c.PodLister, c.TrafficTargetLister, c.HTTPRouteGroupLister, c.TCPRouteLister, c.TrafficSplitLister)

		return nil
	}

	// If SMI is not configured, use the kubernetes provider.
	c.provider = kubernetes.New(c.defaultMode, c.tcpStateTable, c.ignored, c.ServiceLister, c.EndpointsLister)

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

	if err = c.createMeshServices(); err != nil {
		log.Errorf("could not create mesh services: %v", err)
	}

	// Start the api, and enable the readiness endpoint
	c.api.Start()

	for {
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-stopCh:
			log.Info("Shutting down workers")
			return nil
		case message := <-c.configRefreshChan:
			// Reload the configuration
			conf, confErr := c.provider.BuildConfig()
			if confErr != nil {
				return confErr
			}

			if message == k8s.ConfigMessageChanForce || !reflect.DeepEqual(c.lastConfiguration.Get(), conf) {
				c.lastConfiguration.Set(conf)

				if deployErr := c.deployConfiguration(conf); deployErr != nil {
					break
				}

				// Configuration successfully deployed, enable readiness in the api.
				c.api.EnableReadiness()
			}
		case <-timer.C:
			log.Debug("Deploying configuration to unready nodes")

			if deployErr := c.deployConfigurationToUnreadyNodes(c.lastConfiguration.Get().(*dynamic.Configuration)); deployErr != nil {
				break
			}

			// Configuration successfully deployed, enable readiness in the api.
			c.api.EnableReadiness()
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
	sel, err := c.ignored.LabelSelector()
	if err != nil {
		return fmt.Errorf("unable to build label selectors: %w", err)
	}

	// Because createMeshServices is called after startInformers,
	// then we already have the cache built, so we can use it.
	svcs, err := c.ServiceLister.List(sel)
	if err != nil {
		return fmt.Errorf("unable to get services: %w", err)
	}

	for _, service := range svcs {
		if c.ignored.IsIgnored(service.ObjectMeta) {
			continue
		}

		log.Debugf("Creating mesh for service: %v", service.Name)

		meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)

		_, err := c.ServiceLister.Services(c.meshNamespace).Get(meshServiceName)
		if err == nil {
			continue
		}
		// We're expecting an IsNotFound error here, to only create the maesh service if it does not exist.
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("unable to check if maesh service exists: %w", err)
		}

		log.Infof("Creating associated mesh service: %s", meshServiceName)

		if err := c.createMeshService(service); err != nil {
			return fmt.Errorf("unable to create mesh service: %w", err)
		}
	}

	return nil
}

func (c *Controller) createMeshService(service *corev1.Service) error {
	meshServiceName := c.userServiceToMeshServiceName(service.Name, service.Namespace)
	log.Debugf("Creating mesh service: %s", meshServiceName)

	_, err := c.ServiceLister.Services(c.meshNamespace).Get(meshServiceName)
	// We're expecting an IsNotFound error here, to only create the maesh service if it does not exist.
	if err != nil && errors.IsNotFound(err) {
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
				Labels: map[string]string{
					"app": "maesh",
				},
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

	_, err := c.ServiceLister.Services(c.meshNamespace).Get(meshServiceName)
	if err != nil {
		return err
	}

	// Service exists, delete
	if err := c.clients.DeleteService(c.meshNamespace, meshServiceName); err != nil {
		return err
	}

	log.Debugf("Deleted service: %s/%s", c.meshNamespace, meshServiceName)

	return nil
}

// updateMeshService updates the mesh service based on an old/new user service, and returns the updated mesh service
func (c *Controller) updateMeshService(oldUserService *corev1.Service, newUserService *corev1.Service) (*corev1.Service, error) {
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	meshServiceName := c.userServiceToMeshServiceName(oldUserService.Name, oldUserService.Namespace)

	var updatedSvc *corev1.Service

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, err := c.ServiceLister.Services(c.meshNamespace).Get(meshServiceName)
		if err != nil {
			return err
		}

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
	return fmt.Sprintf("%s-%s-6d61657368-%s", c.meshNamespace, serviceName, namespace)
}

func (c *Controller) loadTCPStateTable() (*k8s.State, error) {
	result := c.tcpStateTable
	if result == nil {
		result = &k8s.State{Table: make(map[int]*k8s.ServiceWithPort)}
	}

	configMap, err := c.ConfigMapLister.ConfigMaps(c.meshNamespace).Get(k8s.TCPStateConfigMapName)
	if err != nil {
		return result, err
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
	configMap, err := c.ConfigMapLister.ConfigMaps(c.meshNamespace).Get(k8s.TCPStateConfigMapName)
	if err != nil {
		return err
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
	sel := labels.Everything()

	r, err := labels.NewRequirement("component", selection.Equals, []string{"maesh-mesh"})
	if err != nil {
		return err
	}

	sel = sel.Add(*r)

	podList, err := c.PodLister.Pods(c.meshNamespace).List(sel)
	if err != nil {
		return fmt.Errorf("unable to get pods: %w", err)
	}

	if len(podList) == 0 {
		return fmt.Errorf("unable to find any active mesh pods to deploy config : %+v", config)
	}

	if err := c.deployToPods(podList, config); err != nil {
		return fmt.Errorf("error deploying configuration: %v", err)
	}

	return nil
}

// deployConfigurationToUnreadyNodes deploys the configuration to the mesh pods.
func (c *Controller) deployConfigurationToUnreadyNodes(config *dynamic.Configuration) error {
	sel := labels.Everything()

	r, err := labels.NewRequirement("component", selection.Equals, []string{"maesh-mesh"})
	if err != nil {
		return err
	}

	sel = sel.Add(*r)

	podList, err := c.PodLister.Pods(c.meshNamespace).List(sel)
	if err != nil {
		return fmt.Errorf("unable to get pods: %w", err)
	}

	if len(podList) == 0 {
		return fmt.Errorf("unable to find any active mesh pods to deploy config : %+v", config)
	}

	var unreadyPods []*corev1.Pod

	for _, pod := range podList {
		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				unreadyPods = append(unreadyPods, pod)
				break
			}
		}
	}

	if err := c.deployToPods(unreadyPods, config); err != nil {
		return fmt.Errorf("error deploying configuration: %v", err)
	}

	return nil
}

func (c *Controller) deployToPods(pods []*corev1.Pod, config *dynamic.Configuration) error {
	var errg errgroup.Group

	for _, p := range pods {
		pod := p

		log.Debugf("Deploying to pod %s with IP %s", pod.Name, pod.Status.PodIP)

		errg.Go(func() error {
			b := backoff.NewExponentialBackOff()
			b.MaxElapsedTime = 15 * time.Second

			op := func() error {
				return c.deployToPod(pod.Name, pod.Status.PodIP, config)
			}

			return backoff.Retry(safe.OperationWithRecover(op), b)
		})
	}

	return errg.Wait()
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
			c.deployLog.LogDeploy(time.Now(), name, ip, false, fmt.Sprintf("unable to read response body: %v", bodyErr))
			return fmt.Errorf("unable to read response body: %v", bodyErr)
		}

		if resp.StatusCode != http.StatusOK {
			c.deployLog.LogDeploy(time.Now(), name, ip, false, fmt.Sprintf("received non-ok response code: %d", resp.StatusCode))
			return fmt.Errorf("received non-ok response code: %d", resp.StatusCode)
		}
	}

	if err != nil {
		c.deployLog.LogDeploy(time.Now(), name, ip, false, fmt.Sprintf("unable to deploy configuration: %v", err))
		return fmt.Errorf("unable to deploy configuration: %v", err)
	}

	c.deployLog.LogDeploy(time.Now(), name, ip, true, "")
	log.Debugf("Successfully deployed configuration to pod (%s:%s)", name, ip)

	return nil
}

// isMeshPod checks if the pod is a mesh pod. Can be modified to use multiple metrics if needed.
func isMeshPod(pod *corev1.Pod) bool {
	return pod.Labels["component"] == "maesh-mesh"
}
