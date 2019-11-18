package configurator

import (
	"context"

	"github.com/containous/maesh/internal/providers/base"
	"github.com/containous/maesh/internal/resource"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const moduleName = "configurator"

// Deployer deploys a confiugration to a set of pods.
type Deployer interface {
	Deploy(pods []*corev1.Pod, cfg *dynamic.Configuration) error
}

// Controller is in charge of watching meaningfull resources and triggering mesh node reconfiguration.
type Controller struct {
	provider base.Provider
	deployer Deployer
	pods     listersv1.PodLister

	currentNamespace string

	queue  workqueue.RateLimitingInterface
	exited chan struct{}

	logger logrus.FieldLogger
}

// NewController returns a configurator controller.
func NewController(informerFactory informers.SharedInformerFactory, provider base.Provider, deployer Deployer, currentNamespace string, l logrus.FieldLogger) *Controller {
	q := workqueue.NewNamedRateLimitingQueue(
		// TODO tweak this rate limiting
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 2)},
		moduleName,
	)

	// Watch the mesh-services
	svcInformer := informerFactory.Core().V1().Services()
	svcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { enqueueSvcEvent(q, obj, l) },
		UpdateFunc: func(_, obj interface{}) { enqueueSvcEvent(q, obj, l) },
		DeleteFunc: func(obj interface{}) { enqueueSvcEvent(q, obj, l) },
	})

	// Watch all the endpoints.
	endpointsInformer := informerFactory.Core().V1().Endpoints()
	endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) { q.AddRateLimited(struct{}{}) },
		// We rate limit the update func on endpoints, which can generate a lot of noise, when deploying pods.
		UpdateFunc: func(_, obj interface{}) { q.AddRateLimited(struct{}{}) },
		DeleteFunc: func(_ interface{}) { q.AddRateLimited(struct{}{}) },
	})

	// Watch the mesh nodes pods.
	podsInformer := informerFactory.Core().V1().Pods()
	podsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { enqueuePodEvent(q, obj, l) },
		UpdateFunc: func(_, obj interface{}) { enqueuePodEvent(q, obj, l) },
		DeleteFunc: func(obj interface{}) { enqueuePodEvent(q, obj, l) },
	})

	return &Controller{
		deployer:         deployer,
		provider:         provider,
		pods:             podsInformer.Lister(),
		queue:            q,
		exited:           make(chan struct{}),
		currentNamespace: currentNamespace,
		logger:           l.WithField("module", moduleName),
	}
}

// Run process all events coming from the workqueue until shutdown is signaled.
func (c *Controller) Run() {
	defer close(c.exited)

	for c.processEvent() {
	}
}

// ShutDown shutdowns the controller.
func (c *Controller) ShutDown() {
	c.queue.ShutDown()
}

// Wait waits for the termination of the controller loop.
func (c *Controller) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.exited:
		return nil
	}
}

func (c *Controller) processEvent() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	defer c.queue.Done(item)

	cfg, err := c.provider.BuildConfig()
	if err != nil {
		c.handleError(item, err)
		return true
	}

	maeshNodes, err := c.pods.Pods(c.currentNamespace).List(resource.MeshPodsLabelsSelector())
	if err != nil {
		c.handleError(item, err)
		return true
	}

	if err = c.deployer.Deploy(maeshNodes, cfg); err != nil {
		c.handleError(item, err)
		return true
	}

	c.queue.Forget(item)

	return true
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

func enqueueSvcEvent(q workqueue.RateLimitingInterface, obj interface{}, log logrus.FieldLogger) {
	isMeshSvc, err := resource.IsMeshService(obj)
	if err != nil {
		log.WithError(err).Error("unable to determine if event comes from a mesh service, skipping")
		return
	}
	if !isMeshSvc {
		return
	}

	q.AddRateLimited(struct{}{})
}

func enqueuePodEvent(q workqueue.RateLimitingInterface, obj interface{}, log logrus.FieldLogger) {
	isMeshPod, err := resource.IsMeshPod(obj)
	if err != nil {
		log.WithError(err).Error("unable to determine if event comes from a mesh pod, skipping")
		return
	}
	if !isMeshPod {
		return
	}

	q.AddRateLimited(struct{}{})
}
