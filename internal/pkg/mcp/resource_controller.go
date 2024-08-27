package mcp

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const maxRetries = 5

type Handler interface {
	Init() error
	ObjectCreated(obj runtime.Object)
	ObjectDeleted(obj runtime.Object)
	ObjectUpdated(oldObj, newObj runtime.Object)
}

// Event indicate the informerEvent
type Event struct {
	key          string
	eventType    string
	namespace    string
	resourceType string
	obj          runtime.Object
	oldObj       runtime.Object
}

// Controller object
type Controller struct {
	clientset           kubernetes.Interface
	queue               workqueue.RateLimitingInterface
	informer            cache.SharedIndexInformer
	handlerRegistration cache.ResourceEventHandlerRegistration
	resourceType        string
	eventHandlers       []Handler
}

func objName(obj interface{}) string {
	return reflect.TypeOf(obj).Name()
}

func NewResourceController(client kubernetes.Interface, informer cache.SharedIndexInformer, resourceType interface{}, eventHandlers []Handler) (*Controller, error) {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent Event
	var err error
	handlerRegistration, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"
			newEvent.resourceType = objName(resourceType)
			newEvent.obj = obj.(runtime.Object)
			log.Infof("Processing add to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
			newEvent.eventType = "update"
			newEvent.resourceType = objName(resourceType)
			newEvent.obj = new.(runtime.Object)
			newEvent.oldObj = old.(runtime.Object)
			log.Infof("Processing update to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"
			newEvent.resourceType = objName(resourceType)
			newEvent.obj = obj.(runtime.Object)
			log.Infof("Processing delete to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
	})

	if err != nil {
		return &Controller{}, err
	}

	return &Controller{
		clientset:           client,
		informer:            informer,
		handlerRegistration: handlerRegistration,
		queue:               queue,
		resourceType:        objName(resourceType),
		eventHandlers:       eventHandlers,
	}, nil
}

// Run starts the controller
func (c *Controller) Run(stopCh <-chan struct{}, informersInitGroup *sync.WaitGroup) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	log.Infof("starting %s controller", c.resourceType)

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for %s controller caches to sync", c.resourceType))
		return
	}

	log.Infof("%s controller synced and ready", c.resourceType)

	informersInitGroup.Done()

	for _, handler := range c.eventHandlers {
		handler.Init()
	}

	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced() && // store has been informed by at least one full LIST
		c.handlerRegistration.HasSynced() // and all pre-sync events have been delivered
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (c *Controller) LastSyncResourceVersion() string {
	return c.informer.LastSyncResourceVersion()
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	newEvent, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(newEvent)
	err := c.processItem(newEvent.(Event))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		log.Warnf("Error processing %s (will retry): %v", newEvent.(Event).key, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		log.Errorf("Error processing %s (giving up): %v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(newEvent Event) error {
	// process events based on its type
	switch newEvent.eventType {
	case "create":
		for _, handler := range c.eventHandlers {
			handler.ObjectCreated(newEvent.obj)
		}
	case "update":
		for _, handler := range c.eventHandlers {
			handler.ObjectUpdated(newEvent.oldObj, newEvent.obj)
		}
	case "delete":
		for _, handler := range c.eventHandlers {
			handler.ObjectDeleted(newEvent.obj)
		}
	}
	return nil
}
