package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/containous/i3o/utils"
	traefik_v1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	clients        *utils.ClientWrapper
	queue          workqueue.RateLimitingInterface
	informer       cache.SharedIndexInformer
	handler        Handler
	controllerType string
}

// New is used to build the informers and other required components of the controller,
// and return an initialized controller object
func NewController(clients *utils.ClientWrapper, controllerType interface{}, ignoredNamespaces []string, handler Handler) *Controller {
	var lw *cache.ListWatch
	var ot runtime.Object
	var printableType string
	switch controllerType.(type) {
	case apiv1.Service:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the services (core resource) in all namespaces
				return clients.KubeClient.CoreV1().Services(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the services (core resource) in all namespaces
				return clients.KubeClient.CoreV1().Services(metav1.NamespaceAll).Watch(options)
			},
		}
		ot = &apiv1.Service{}
		printableType = "service"
	case apiv1.Endpoints:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the endpoints (core resource) in all namespaces
				return clients.KubeClient.CoreV1().Endpoints(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the endpoints (core resource) in all namespaces
				return clients.KubeClient.CoreV1().Endpoints(metav1.NamespaceAll).Watch(options)
			},
		}
		ot = &apiv1.Endpoints{}
		printableType = "endpoint"

	case apiv1.Namespace:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the namespaces
				return clients.KubeClient.CoreV1().Namespaces().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the namespaces
				return clients.KubeClient.CoreV1().Namespaces().Watch(options)
			},
		}
		ot = &apiv1.Namespace{}
		printableType = "namespace"

	case traefik_v1alpha1.IngressRoute:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the namespaces
				return clients.CrdClient.TraefikV1alpha1().IngressRoutes(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the namespaces
				return clients.CrdClient.TraefikV1alpha1().IngressRoutes(metav1.NamespaceAll).Watch(options)
			},
		}
		ot = &traefik_v1alpha1.IngressRoute{}
		printableType = "ingressroute"
	}

	informer := cache.NewSharedIndexInformer(
		// the ListWatch contains two different functions that our
		// informer requires: ListFunc to take care of listing and watching
		// the resources we want to handle
		lw,
		ot, // the target type
		0,  // no resync (period of 0)
		cache.Indexers{},
	)

	// create a new queue so that when the informer gets a resource that is either
	// a result of listing or watching, we can add an idenfitying key to the queue
	// so that it can be handled in the handler
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// convert the resource object into a key (in this case
			// we are just doing it in the format of 'namespace/name')
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				// add the key to the queue for the handler to get
				// If object key is not in our list of ignored namespaces
				if !ObjectKeyInNamespace(key, ignoredNamespaces) {
					log.Warnf("%s informer - Added: %s to queue", printableType, key)
					queue.Add(key)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				if !ObjectKeyInNamespace(key, ignoredNamespaces) {
					log.Warnf("%s informer - Update: %s", printableType, key)
					queue.Add(key)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				if !ObjectKeyInNamespace(key, ignoredNamespaces) {
					log.Warnf("%s informer - Delete: %s", printableType, key)
					queue.Add(key)
				}
			}
		},
	})

	return &Controller{
		clients:        clients,
		informer:       informer,
		queue:          queue,
		handler:        handler,
		controllerType: printableType,
	}

}

// Run is the main entrypoint for the controller
func (c *Controller) Run(stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.queue.ShutDown()

	log.Infof("Initializing %s controller", c.controllerType)

	// run the informer to start listing and watching resources
	go c.informer.Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("error syncing cache"))
		return
	}
	log.Infof("Controller.%s.Run: cache sync complete", c.controllerType)

	// run the runWorker method every second with a stop channel
	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced allows us to satisfy the Controller interface
// by wiring up the informer's HasSynced method to it
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	log.Debugf("Controller.%s.runWorker: starting", c.controllerType)

	// invoke processNextItem to fetch and consume the next change
	// to a watched or listed resource
	for c.processNextItem() {
		log.Debugf("Controller.%s.runWorker: processing next item", c.controllerType)
	}

	log.Debugf("Controller.%s.runWorker: completed", c.controllerType)
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of if the item was
// created or deleted
func (c *Controller) processNextItem() bool {
	log.Debugf("Controller.%s Waiting for next item to process...", c.controllerType)

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	key, quit := c.queue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.queue.Done(key)

	// assert the string out of the key (format `namespace/name`)
	keyRaw := key.(string)

	// take the string key and get the object out of the indexer
	//
	// item will contain the complex object for the resource and
	// exists is a bool that'll indicate whether or not the
	// resource was created (true) or deleted (false)
	//
	// if there is an error in getting the key from the index
	// then we want to retry this particular queue key a certain
	// number of times (5 here) before we forget the queue key
	// and throw an error
	item, exists, err := c.informer.GetIndexer().GetByKey(keyRaw)
	if err != nil {
		if c.queue.NumRequeues(key) < 5 {
			log.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, retrying", key, err)
			c.queue.AddRateLimited(key)
		} else {
			log.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, no more retries", key, err)
			c.queue.Forget(key)
			utilruntime.HandleError(err)
		}
	}

	// if the item doesn't exist then it was deleted and we need to fire off the handler's
	// ObjectDeleted method. but if the object does exist that indicates that the object
	// was created (or updated) so run the ObjectCreated method
	//
	// after both instances, we want to forget the key from the queue, as this indicates
	// a code path of successful queue key processing
	if !exists {
		log.Infof("Controller.%s.processNextItem: deleted: %s", c.controllerType, keyRaw)
		c.handler.ObjectDeleted(item)
		c.queue.Forget(key)
	} else {
		log.Infof("Controller.%s.processNextItem: created: %s", c.controllerType, keyRaw)
		c.handler.ObjectCreated(item)
		c.queue.Forget(key)
	}

	if c.queue.Len() > 0 {
		// keep the worker loop running by returning true
		return true
	}
	return false
}

func ObjectKeyInNamespace(key string, namespaces []string) bool {
	splitKey := strings.Split(key, "/")
	if len(splitKey) == 1 {
		// No namespace in the key
		return false
	}

	return utils.Contains(namespaces, splitKey[0])
}
