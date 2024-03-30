package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	resourceapi "k8s.io/api/resource/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	fakecrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
)

func StartClaimParametersGenerator(ctx context.Context, config *Config) error {
	logger := klog.FromContext(ctx)

	csconfig, err := GetClientsetConfig(ctx, config.flags)
	if err != nil {
		return fmt.Errorf("error creating client set config: %w", err)
	}

	// Create a new dynamic client
	dynamicClient, err := dynamic.NewForConfig(csconfig)
	if err != nil {
		return fmt.Errorf("error creating dynamic client: %w", err)
	}

	logger.Info("Starting ResourceClaimParameters generator")

	// Set up informer to watch for FakeClaimParameters objects
	fakeClaimParametersInformer := newFakeClaimParametersInformer(dynamicClient)

	// Set up handler for events
	fakeClaimParametersInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			unstructured := obj.(*unstructured.Unstructured)

			var fakeClaimParameters fakecrd.FakeClaimParameters
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, &fakeClaimParameters)
			if err != nil {
				klog.Errorf("Error converting *unstructured.Unstructured to FakeClaimParameters: %v", err)
				return
			}

			if err := createOrUpdateResourceClaimParameters(config.clientset.core, &fakeClaimParameters); err != nil {
				klog.Errorf("Error creating ResourceClaimParameters: %v", err)
				return
			}
		},
		UpdateFunc: func(oldObj any, newObj any) {
			unstructured := newObj.(*unstructured.Unstructured)

			var fakeClaimParameters fakecrd.FakeClaimParameters
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, &fakeClaimParameters)
			if err != nil {
				klog.Errorf("Error converting *unstructured.Unstructured to FakeClaimParameters: %v", err)
				return
			}

			if err := createOrUpdateResourceClaimParameters(config.clientset.core, &fakeClaimParameters); err != nil {
				klog.Errorf("Error updating ResourceClaimParameters: %v", err)
				return
			}
		},
	})

	// Start informer
	go fakeClaimParametersInformer.Run(ctx.Done())

	return nil
}

func GetClientsetConfig(ctx context.Context, f *Flags) (*rest.Config, error) {
	logger := klog.FromContext(ctx)
	var csconfig *rest.Config

	kubeconfigEnv := os.Getenv("KUBECONFIG")
	if kubeconfigEnv != "" {
		logger.Info("Found KUBECONFIG environment variable set, using that...")
		*f.kubeconfig = kubeconfigEnv
	}

	var err error
	if *f.kubeconfig == "" {
		csconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("create in-cluster client configuration: %v", err)
		}
	} else {
		csconfig, err = clientcmd.BuildConfigFromFlags("", *f.kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("create out-of-cluster client configuration: %v", err)
		}
	}

	csconfig.QPS = *f.kubeAPIQPS
	csconfig.Burst = *f.kubeAPIBurst

	return csconfig, nil
}

func newFakeClaimParametersInformer(dynamicClient dynamic.Interface) cache.SharedIndexInformer {
	// Set up shared index informer for FakeClaimParameters objects
	gvr := schema.GroupVersionResource{
		Group:    fakecrd.GroupName,
		Version:  fakecrd.Version,
		Resource: strings.ToLower(fakecrd.FakeClaimParametersKind),
	}

	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dynamicClient.Resource(gvr).List(context.Background(), metav1.ListOptions{})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dynamicClient.Resource(gvr).Watch(context.Background(), metav1.ListOptions{})
			},
		},
		&unstructured.Unstructured{},
		0, // resyncPeriod
		cache.Indexers{},
	)

	return informer
}

func createOrUpdateResourceClaimParameters(clientset kubernetes.Interface, fakeClaimParameters *fakecrd.FakeClaimParameters) error {
	namespace := fakeClaimParameters.Namespace

	// Get a list of existing ResourceClaimParameters in the same namespace as the incoming FakeClaimParameters
	existing, err := clientset.ResourceV1alpha2().ResourceClaimParameters(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error listing existing ResourceClaimParameters: %w", err)
	}

	// Build a new ResourceClaimParameters object from the incoming FakeClaimParameters object
	resourceClaimParameters, err := newResourceClaimParametersFromFakeClaimParameters(fakeClaimParameters)
	if err != nil {
		return fmt.Errorf("error building new ResourceClaimParameters object from a FakeClaimParameters object: %w", err)
	}

	// If there is an existing ResourceClaimParameters generated from the incoming FakeClaimParameters object, then update it
	if len(existing.Items) > 0 {
		for _, item := range existing.Items {
			if (item.GeneratedFrom.APIGroup == fakecrd.GroupName) &&
				(item.GeneratedFrom.Kind == fakeClaimParameters.Kind) &&
				(item.GeneratedFrom.Name == fakeClaimParameters.Name) {
				klog.Infof("ResourceClaimParameters already exists for FakeClaimParameters %s/%s, updating it", namespace, fakeClaimParameters.Name)

				// Copy the matching ResourceClaimParameters metadata into the new ResourceClaimParameters object before updating it
				resourceClaimParameters.ObjectMeta = *item.ObjectMeta.DeepCopy()

				_, err = clientset.ResourceV1alpha2().ResourceClaimParameters(namespace).Update(context.TODO(), resourceClaimParameters, metav1.UpdateOptions{})
				if err != nil {
					return fmt.Errorf("error updating ResourceClaimParameters object: %w", err)
				}

				return nil
			}
		}
	}

	// Otherwise create a new ResourceClaimParameters object from the incoming FakeClaimParameters object
	_, err = clientset.ResourceV1alpha2().ResourceClaimParameters(namespace).Create(context.TODO(), resourceClaimParameters, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating ResourceClaimParameters object from FakeClaimParameters object: %w", err)
	}

	klog.Infof("Created ResourceClaimParameters for FakeClaimParameters %s/%s", namespace, fakeClaimParameters.Name)
	return nil
}

func newResourceClaimParametersFromFakeClaimParameters(fakeClaimParameters *fakecrd.FakeClaimParameters) (*resourceapi.ResourceClaimParameters, error) {
	namespace := fakeClaimParameters.Namespace

	rawSpec, err := json.Marshal(fakeClaimParameters.Spec)
	if err != nil {
		return nil, fmt.Errorf("error marshaling FakeClaimParamaters to JSON: %w", err)
	}

	resourceCount := 1
	if fakeClaimParameters.Spec.Count != 0 {
		resourceCount = fakeClaimParameters.Spec.Count
	}

	selector := "true"
	if fakeClaimParameters.Spec.Selector != nil {
		selector = fakeClaimParameters.Spec.Selector.ToNamedResourcesSelector()
	}

	shareable := true

	var resourceRequests []resourceapi.ResourceRequest
	// split を指定していても Fake リソースを動的に分割するためリソース要求に
	// count で指定した個数だけ準備すれば良い
	for i := 0; i < resourceCount; i++ {
		resourceRequests = append(resourceRequests, resourceapi.ResourceRequest{
			ResourceRequestModel: resourceapi.ResourceRequestModel{
				NamedResources: &resourceapi.NamedResourcesRequest{
					Selector: selector,
				},
			},
		})
	}

	resourceClaimParameters := &resourceapi.ResourceClaimParameters{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "resource-claim-parameters-",
			Namespace:    namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         fakeClaimParameters.APIVersion,
					Kind:               fakeClaimParameters.Kind,
					Name:               fakeClaimParameters.Name,
					UID:                fakeClaimParameters.UID,
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		GeneratedFrom: &resourceapi.ResourceClaimParametersReference{
			APIGroup: fakecrd.GroupName,
			Kind:     fakeClaimParameters.Kind,
			Name:     fakeClaimParameters.Name,
		},
		DriverRequests: []resourceapi.DriverRequests{
			{
				DriverName:       DriverName,
				VendorParameters: runtime.RawExtension{Raw: rawSpec},
				Requests:         resourceRequests,
			},
		},
		Shareable: shareable,
	}

	return resourceClaimParameters, nil
}
