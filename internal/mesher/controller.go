package mesher

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Filter is a type that can match a k8s object.
type Filter interface {
	Match(metav1.Object) bool
}

const moduleName = "mesher"

// Controller is running the control loop to maintain the list of mesh services.
type Controller struct {
	svcCache  listersv1.ServiceLister
	svcClient clientv1.ServiceInterface

	queue  workqueue.RateLimitingInterface
	exited chan struct{}

	currentNamespace string

	logger logrus.FieldLogger
}

// NewController setup event handlers on the service informer and returns a controller.
func NewController(clientSet kubernetes.Interface, informerFactory informers.SharedInformerFactory, ns string, l logrus.FieldLogger) *Controller {
	q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), moduleName)

	svcInformer := informerFactory.Core().V1().Services()

	svcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { enqueueObj(q, l, typeAdd, obj) },
		UpdateFunc: func(_, obj interface{}) { enqueueObj(q, l, typeUpdate, obj) },
		DeleteFunc: func(obj interface{}) { enqueueObj(q, l, typeDelete, obj) },
	})

	return &Controller{
		svcCache:         svcInformer.Lister(),
		svcClient:        clientSet.CoreV1().Services(ns),
		queue:            q,
		exited:           make(chan struct{}),
		currentNamespace: ns,
		logger:           l.WithField("module", moduleName),
	}
}

// Run process all events coming from the workqueue until shutdown is signaled.
func (c *Controller) Run() {
	defer close(c.exited)

	for c.processEvent() {
	}
}

func (c *Controller) processEvent() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	defer c.queue.Done(item)

	evt := item.(event)

	var err error

	switch evt.Type {
	case typeAdd:
		err = c.createMeshService(evt.Key)
	case typeUpdate:
		err = c.updateMeshService(evt.Key)
	case typeDelete:
		err = c.deleteMeshService(evt.Key)
	default:
		c.logger.Errorf("Unknown event type %q, ignoring", evt.Type)
		c.queue.Forget(item)
		return true
	}

	if err != nil {
		c.handleError(item, err)
		return true
	}

	c.queue.Forget(item)
	return true
}

// ShutDown shutdowns the controller.
func (c *Controller) ShutDown() {
	c.queue.ShutDown()
}

func (c *Controller) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.exited:
		return nil
	}
}

const (
	maxAttempts = 3
)

func (c *Controller) handleError(item interface{}, err error) {
	l := c.logger.WithError(err)

	attempt := c.queue.NumRequeues(item)
	if attempt < maxAttempts {
		l.Errorf("(%d/%d) unable to process item, retrying", attempt+1, maxAttempts)
		c.queue.AddRateLimited(item)
		return
	}

	l.Errorf("Max attempt reached, ignoring failing event")
	c.queue.Forget(item)
}

func (c *Controller) createMeshService(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("unable to parse key: %w", err)
	}

	addedSvc, err := c.svcCache.Services(ns).Get(name)
	if err != nil {
		return fmt.Errorf("unable to get added svc from cache: %v", err)
	}

	meshSvcName := meshServiceName(name, ns, c.currentNamespace)

	_, err = c.svcCache.Services(c.currentNamespace).Get(meshSvcName)
	if err == nil {
		// If there is a maesh service already present, then we try to update it.
		return c.updateMeshService(key)
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("unable to lookup for maesh service: %w", err)
	}

	_, err = c.svcClient.Create(c.buildMeshSvc(meshSvcName, addedSvc))
	if err != nil {
		return fmt.Errorf("unable to create mesh service: %v", err)
	}

	c.logger.Infof(
		"Created mesh service %q, from created service %q from namespace %q",
		meshSvcName,
		name,
		ns,
	)

	return nil
}

func (c *Controller) updateMeshService(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("unable to parse key: %w", err)
	}

	updatedService, err := c.svcCache.Services(ns).Get(name)
	if err != nil {
		return fmt.Errorf("unable to get updated svc from cache: %v", err)
	}

	meshSvcName := meshServiceName(name, ns, c.currentNamespace)

	_, err = c.svcClient.Update(c.buildMeshSvc(meshSvcName, updatedService))
	if err != nil {
		return fmt.Errorf("unable to update mesh service: %v", err)
	}

	c.logger.Infof(
		"Updated mesh service %q, from an updated of service %q from namespace %q",
		meshSvcName,
		name,
		ns,
	)

	return nil
}

func (c *Controller) deleteMeshService(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("unable to parse key: %w", err)
	}

	meshSvcName := meshServiceName(name, ns, c.currentNamespace)

	if err = c.svcClient.Delete(meshSvcName, nil); err != nil {
		return fmt.Errorf("unable to delete mesh service: %v", err)
	}

	c.logger.Infof(
		"Deleted mesh service %q, because of deletion of service %q from namespace %q",
		meshSvcName,
		name,
		ns,
	)

	return nil
}

const (
	portRangeStart = 5000

	appLabel       = "app"
	componentLabel = "component"

	appLabelMaesh          = "maesh"
	componentLabelMeshSvc  = "mesh-svc"
	componentLabelMeshNode = "mesh-node"
)

func (c *Controller) buildMeshSvc(name string, sourceSvc *corev1.Service) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.currentNamespace,
			Labels: map[string]string{
				appLabel:       appLabelMaesh,
				componentLabel: componentLabelMeshSvc,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				appLabel:       appLabelMaesh,
				componentLabel: componentLabelMeshNode,
			},
		},
	}

	for id, port := range sourceSvc.Spec.Ports {
		port := corev1.ServicePort{
			Name:       port.Name,
			Protocol:   port.Protocol,
			Port:       port.Port,
			TargetPort: intstr.FromInt(portRangeStart + id),
		}

		svc.Spec.Ports = append(svc.Spec.Ports, port)
	}

	return svc
}

func meshServiceName(name, ns, currentNs string) string {
	return fmt.Sprintf("%s-%s-%s", currentNs, name, ns)
}

func isMeshService(obj interface{}) (bool, error) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	objLabels := objMeta.GetLabels()

	isMaeshService := objLabels[appLabel] == appLabelMaesh
	isMeshSvc := objLabels[componentLabel] == componentLabelMeshSvc

	return isMaeshService && isMeshSvc, nil
}

func enqueueObj(q workqueue.RateLimitingInterface, log logrus.FieldLogger, t eventType, obj interface{}) {
	isMeshSvc, err := isMeshService(obj)
	if err != nil {
		log.WithError(err).Error("unable to detect mesh service from event, ignoring")
		return
	}

	if isMeshSvc {
		return
	}

	var key string

	if t == typeDelete {
		key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	} else {
		key, err = cache.MetaNamespaceKeyFunc(obj)
	}

	if err != nil {
		log.Errorf("ignoring item, unable to calculate meta namespace key: %v", err)
		return
	}

	q.Add(event{Type: t, Key: key})
}

type eventType int8

const (
	typeUnknown eventType = iota
	typeAdd
	typeUpdate
	typeDelete
)

type event struct {
	Type eventType
	Key  string
}
