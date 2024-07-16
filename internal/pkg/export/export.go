package export

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func InitWatcher(ctx context.Context, clientset *kubernetes.Clientset) error {
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	serviceInformer := informerFactory.Core().V1().Services().Informer()
	_, err := serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			fmt.Printf("Service added: %v\n", svc.Name)
			//if matchesLabelSelector(svc, labelSelector) {
			//	fmt.Printf("Service added: %s/%s\n", svc.Namespace, svc.Name)
			//}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			svc := newObj.(*corev1.Service)
			fmt.Printf("Service updated: %v\n", svc.Name)
			//if matchesLabelSelector(svc, labelSelector) {
			//	fmt.Printf("Service updated: %s/%s\n", svc.Namespace, svc.Name)
			//}
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			fmt.Printf("Service deleted: %v\n", svc.Name)
			//if matchesLabelSelector(svc, labelSelector) {
			//	fmt.Printf("Service deleted: %s/%s\n", svc.Namespace, svc.Name)
			//}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add service informer: %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	informerFactory.Start(stopCh)
	if ok := cache.WaitForCacheSync(stopCh, serviceInformer.HasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync: %v", err)
	}

	<-ctx.Done()
	return nil
}

//func matchesLabelSelector(obj *corev1.Service, labelSelector string) bool {
//	selector, err := metav1.ParseToLabelSelector(labelSelector)
//	if err != nil {
//		klog.Errorf("Error parsing label selector: %s", err.Error())
//		return false
//	}
//	return labels.SelectorFromSet(selector.MatchLabels).Matches(labels.Set(obj.GetLabels()))
//}
