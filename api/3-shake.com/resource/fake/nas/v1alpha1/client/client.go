package client

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	nasclient "github.com/toVersus/fake-dra-driver/pkg/3-shake.com/resource/clientset/versioned/typed/nas/v1alpha1"
)

type Client struct {
	nas    *nascrd.NodeAllocationState
	client nasclient.NasV1alpha1Interface
}

func New(nas *nascrd.NodeAllocationState, client nasclient.NasV1alpha1Interface) *Client {
	return &Client{
		nas:    nas,
		client: client,
	}
}

func (c *Client) Get() error {
	crd, err := c.client.NodeAllocationStates(c.nas.Namespace).Get(context.TODO(), c.nas.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get NodeAllocationState resource %q of %q: %w", c.nas.Name, c.nas.Kind, err)
	}
	*c.nas = *crd
	return nil
}

func (c *Client) GetOrCreate() error {
	err := c.Get()
	if err == nil {
		return nil
	}
	if errors.IsNotFound(err) {
		klog.InfoS("NodeAllocationState not found. Creating new one", "NodeAllocationState", c.nas.Name, "namespace", c.nas.Namespace)
		return c.Create()
	}
	return err
}

func (c *Client) Create() error {
	crd := c.nas.DeepCopy()
	crd, err := c.client.NodeAllocationStates(c.nas.Namespace).Create(context.TODO(), crd, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create NodeAllocationState resource %q of %q: %w", c.nas.Name, c.nas.Kind, err)
	}
	*c.nas = *crd
	return nil
}

func (c *Client) Delete() error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}
	err := c.client.NodeAllocationStates(c.nas.Namespace).Delete(context.TODO(), c.nas.Name, deleteOptions)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete NodeAllocationState resource %q of %q: %w", c.nas.Name, c.nas.Kind, err)
	}
	return nil
}

func (c *Client) Update(spec *nascrd.NodeAllocationStateSpec) error {
	crd := c.nas.DeepCopy()
	crd.Spec = *spec
	crd, err := c.client.NodeAllocationStates(c.nas.Namespace).Update(context.TODO(), crd, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update NodeAllocationState resource %q of %q: %w", c.nas.Name, c.nas.Kind, err)
	}
	*c.nas = *crd
	return nil
}

func (c *Client) UpdateStatus(status string) error {
	crd := c.nas.DeepCopy()
	crd.Status = status
	crd, err := c.client.NodeAllocationStates(c.nas.Namespace).Update(context.TODO(), crd, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update status of NodeAllocationState resource %q of %q: %w", c.nas.Name, c.nas.Kind, err)
	}
	*c.nas = *crd
	return nil
}
