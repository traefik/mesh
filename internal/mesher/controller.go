package mesher

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	informersv1 "k8s.io/client-go/informers/core/v1"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Filter is a type that can match a k8s object.
type Filter interface {
	Match(metav1.Object) bool
}

// Controller is running the control loop to maintain the list of mesh services.
type Controller struct {
	svcCache  listersv1.ServiceLister
	svcClient clientv1.ServiceInterface

	queue  workqueue.RateLimitingInterface
	exited chan struct{}
}

// NewController setup event handlers on the service informer and returns a controller.
func NewController(client clientv1.ServiceInterface, informer informersv1.ServiceInformer, filter Filter) *Controller {
	q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "mesher")

	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	})

	return &Controller{
		svcCache:  informer.Lister(),
		svcClient: client,
		queue:     q,
		exited:    make(chan struct{}),
	}
}

// Run runs the controller.
func (c *Controller) Run() {
	for {
		evt, shutdown := c.queue.Get()
		if shutdown {
			close(c.exited)
			return
		}

		defer c.queue.Done(evt)

		c.queue.Forget(evt)
	}
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
