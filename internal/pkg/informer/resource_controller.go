// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package informer

import (
	"reflect"
	"time"

	istiolog "istio.io/istio/pkg/log"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const maxRetries = 5

var log = istiolog.RegisterScope("controller", "K8s resource controller")

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

var _ cache.Controller = (*Controller)(nil)

type Controller struct {
	queue               workqueue.RateLimitingInterface
	informer            cache.SharedIndexInformer
	handlerRegistration cache.ResourceEventHandlerRegistration
	resourceType        string
	eventHandlers       []Handler
}

func NewResourceController(informer cache.SharedIndexInformer, resourceType interface{}, eventHandlers ...Handler) (*Controller, error) {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	handlerRegistration, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			var newEvent Event
			var err error
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"
			newEvent.resourceType = objName(resourceType)
			newEvent.obj = obj.(runtime.Object)
			log.Debugf("Processing add to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			var newEvent Event
			var err error
			newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
			newEvent.eventType = "update"
			newEvent.resourceType = objName(resourceType)
			newEvent.obj = new.(runtime.Object)
			newEvent.oldObj = old.(runtime.Object)
			log.Debugf("Processing update to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			var newEvent Event
			var err error
			newEvent.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"
			newEvent.resourceType = objName(resourceType)
			newEvent.obj = obj.(runtime.Object)
			log.Debugf("Processing delete to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
	})

	if err != nil {
		return &Controller{}, err
	}

	return &Controller{
		informer:            informer,
		handlerRegistration: handlerRegistration,
		queue:               queue,
		resourceType:        objName(resourceType),
		eventHandlers:       eventHandlers,
	}, nil
}

func (c *Controller) RunAndWait(stopCh <-chan struct{}) {
	go c.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		log.Fatalf("timed out waiting for %s controller caches to sync", c.resourceType)
	}
}

// Run waits for informer to be synced and starts processing queue
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		log.Fatalf("timed out waiting for %s controller caches to sync", c.resourceType)
	}
	log.Infof("%s controller synced and ready", c.resourceType)

	for _, handler := range c.eventHandlers {
		if err := handler.Init(); err != nil {
			log.Errorf("failed to init event handler: %v", err)
		}
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

func objName(obj interface{}) string {
	return reflect.TypeOf(obj).Name()
}
