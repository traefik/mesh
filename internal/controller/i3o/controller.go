package i3o

import (
	"fmt"
	"strings"
	"time"

	"github.com/containous/i3o/internal/k8s"
	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiSplitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	client               k8s.Client
	queue                workqueue.RateLimitingInterface
	informer             cache.SharedIndexInformer
	handler              Handler
	controllerType       interface{}
	controllerTypeString string
}

// New is used to build the informers and other required components of the controller,
// and return an initialized controller object
func NewController(client k8s.Client, controllerType interface{}, ignored k8s.IgnoreWrapper, handler Handler) *Controller {
	var lw *cache.ListWatch
	var ot runtime.Object
	var printableType string
	switch controllerType.(type) {
	case corev1.Service:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the services (core resource) in all namespaces
				return client.ListServicesWithOptions(metav1.NamespaceAll, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the services (core resource) in all namespaces
				return client.WatchServicesWithOptions(metav1.NamespaceAll, options)
			},
		}
		ot = &corev1.Service{}
		printableType = "service"
	case smiAccessv1alpha1.TrafficTarget:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the traffic targets (SMI access object) in all namespaces
				return client.ListTrafficTargetsWithOptions(metav1.NamespaceAll, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the traffic targets (SMI access object) in all namespaces
				return client.WatchTrafficTargetsWithOptions(metav1.NamespaceAll, options)
			},
		}
		ot = &smiAccessv1alpha1.TrafficTarget{}
		printableType = "traffictarget"

	case smiSpecsv1alpha1.HTTPRouteGroup:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the httproutegroups (SMI specs object) in all namespaces
				return client.ListHTTPRouteGroupsWithOptions(metav1.NamespaceAll, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the httproutegroups (SMI specs object) in all namespaces
				return client.WatchHTTPRouteGroupsWithOptions(metav1.NamespaceAll, options)
			},
		}
		ot = &smiSpecsv1alpha1.HTTPRouteGroup{}
		printableType = "httproutegroup"

	case smiSplitv1alpha1.TrafficSplit:
		lw = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the traffic splits (SMI specs object) in all namespaces
				return client.ListTrafficSplitsWithOptions(metav1.NamespaceAll, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the traffic splits (SMI specs object) in all namespaces
				return client.WatchTrafficSplitsWithOptions(metav1.NamespaceAll, options)
			},
		}
		ot = &smiSplitv1alpha1.TrafficSplit{}
		printableType = "trafficsplit"
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
				if !ObjectKeyInNamespace(key, ignored.Namespaces) {
					log.Warnf("%s informer - Added: %s to queue", printableType, key)
					event := Message{
						Key:    key,
						Object: obj,
						Action: MessageTypeCreated,
					}
					queue.Add(event)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				if !ObjectKeyInNamespace(key, ignored.Namespaces) {
					log.Warnf("%s informer - Update: %s", printableType, key)
					event := Message{
						Key:       key,
						Object:    newObj,
						OldObject: oldObj,
						Action:    MessageTypeUpdated,
					}
					queue.Add(event)
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
				if !ObjectKeyInNamespace(key, ignored.Namespaces) {
					log.Warnf("%s informer - Delete: %s", printableType, key)
					event := Message{
						Key:    key,
						Object: obj,
						Action: MessageTypeDeleted,
					}
					queue.Add(event)
				}
			}
		},
	})

	return &Controller{
		client:               client,
		informer:             informer,
		queue:                queue,
		handler:              handler,
		controllerType:       controllerType,
		controllerTypeString: printableType,
	}

}

// Run is the main entrypoint for the controller
func (c *Controller) Run(stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.queue.ShutDown()

	log.Infof("Initializing %s controller", c.controllerTypeString)

	// run the informer to start listing and watching resources
	go c.informer.Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("error syncing cache"))
		return
	}
	log.Infof("Controller.%s.Run: cache sync complete", c.controllerTypeString)

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
	log.Debugf("Controller.%s.runWorker: starting", c.controllerTypeString)

	// invoke processNextItem to fetch and consume the next change
	// to a watched or listed resource
	for c.processNextItem() {
		log.Debugf("Controller.%s.runWorker: processing next item", c.controllerTypeString)
	}

	log.Debugf("Controller.%s.runWorker: completed", c.controllerTypeString)
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of the event type.
func (c *Controller) processNextItem() bool {
	log.Debugf("Controller.%s Waiting for next item to process...", c.controllerTypeString)

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	item, quit := c.queue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.queue.Done(item)

	event := item.(Message)

	switch event.Action {
	case MessageTypeCreated:
		log.Infof("Controller.%s.processNextItem: created: %s", c.controllerTypeString, event.Key)
		c.handler.ObjectCreated(event)

	case MessageTypeUpdated:
		log.Infof("Controller.%s.processNextItem: updated: %s", c.controllerTypeString, event.Key)
		c.handler.ObjectUpdated(event)

	case MessageTypeDeleted:
		log.Infof("Controller.%s.processNextItem: deleted: %s", c.controllerTypeString, event.Key)
		c.handler.ObjectDeleted(event)
	}

	c.queue.Forget(item)

	// keep the worker loop running by returning true if there are queue objects remaining
	return c.queue.Len() > 0
}

func ObjectKeyInNamespace(key string, namespaces k8s.Namespaces) bool {
	splitKey := strings.Split(key, "/")
	if len(splitKey) == 1 {
		// No namespace in the key
		return false
	}

	return namespaces.Contains(splitKey[0])
}
