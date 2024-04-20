package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1alpha3"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	nasclient "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1/client"
)

var _ drapbv1.NodeServer = &driver{}

type driver struct {
	sync.Mutex

	nascrd    *nascrd.NodeAllocationState
	nasclient *nasclient.Client
	state     *DeviceState
}

func NewDriver(ctx context.Context, config *Config) (*driver, error) {
	logger := klog.FromContext(ctx).WithValues("nodeAllocationState", klog.KObj(config.nascrd))
	ctx = klog.NewContext(ctx, logger)
	var d *driver
	client := nasclient.New(config.nascrd, config.shakeclient.NasV1alpha1())
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.V(4).Info("Creating or using existing NodeAllocationState")
		if err := client.GetOrCreate(); err != nil {
			return err
		}

		logger.V(4).Info("Updating status of NodeAllocationState to NotReady")
		if err := client.UpdateStatus(nascrd.NodeAllocationStateStatusNotReady); err != nil {
			return err
		}

		logger.V(4).Info("Generating mock Fake devices")
		state, err := NewDeviceState(ctx, config)
		if err != nil {
			return err
		}

		logger.V(4).Info("Adding mock Fake devices to allocatableDevice on NodeAllocationState")
		if err := client.Update(state.GetUpdatedSpec(&config.nascrd.Spec)); err != nil {
			return err
		}

		logger.V(4).Info("Updating status of NodeAllocationState to Ready")
		if err := client.UpdateStatus(nascrd.NodeAllocationStateStatusReady); err != nil {
			return err
		}

		d = &driver{
			nascrd:    config.nascrd,
			nasclient: client,
			state:     state,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *driver) Shutdown(ctx context.Context) error {
	logger := klog.FromContext(ctx)
	logger.V(2).Info("Updating status of NodeAllocationState to NotReady before shutting down fake-dra-driver")
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := d.nasclient.Get(); err != nil {
			return err
		}
		return d.nasclient.UpdateStatus(nascrd.NodeAllocationStateStatusNotReady)
	})
}

func (d *driver) NodeListAndWatchResources(req *drapbv1.NodeListAndWatchResourcesRequest, stream drapbv1.Node_NodeListAndWatchResourcesServer) error {
	// DRA Structured Parameters is not supported yet
	return nil
}

func (d *driver) NodePrepareResources(ctx context.Context, req *drapbv1.NodePrepareResourcesRequest) (*drapbv1.NodePrepareResourcesResponse, error) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("NodePrepareResource is called", "numClaims", len(req.Claims))

	preparedResources := &drapbv1.NodePrepareResourcesResponse{Claims: map[string]*drapbv1.NodePrepareResourceResponse{}}

	// In production version some common operations of d.nodeUnprepareResources
	// should be done outside of the loop, for instance updating the CR could
	// be done once after all HW was prepared.
	for _, claim := range req.Claims {
		prepared := d.nodePrepareResource(ctx, claim)
		klog.V(4).Info("Prepared devices for allocated claims", "devices", klog.Format(prepared))
		preparedResources.Claims[claim.Uid] = prepared
	}

	return preparedResources, nil
}

func (d *driver) nodePrepareResource(ctx context.Context, claim *drapbv1.Claim) *drapbv1.NodePrepareResourceResponse {
	logger := klog.FromContext(ctx)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("NodePrepareResource is called")
	d.Lock()
	defer d.Unlock()

	var prepared []string
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.V(4).Info("Getting NodeAllocationState and hold it onto driver struct for preparing resource")
		err := d.nasclient.Get()
		if err != nil {
			return err
		}
		logger.V(4).Info("Preparing devices for claim")
		prepared, err = d.state.Prepare(ctx, claim.Uid, d.nascrd.Spec.AllocatedClaims[claim.Uid])
		if err != nil {
			return fmt.Errorf("error preparing devices for claim %q: %w", claim.Uid, err)
		}

		logger.V(4).Info("Updating spec of NodeAllocationState and add prepared devices to PreparedDevices field")
		if err := d.nasclient.Update(d.state.GetUpdatedSpec(&d.nascrd.Spec)); err != nil {
			if nestedErr := d.state.Unprepare(ctx, claim.Uid); nestedErr != nil {
				logger.Error(errors.Join(err, nestedErr), "Error unpreparing resource after claim Update() failed")
			} else {
				logger.Error(err, "Error updating NodeAllocationState status to NotReady, so unpreparing earlier prepared resource")
			}
			return err
		}

		return nil
	})
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("failed to retry preparing resource: %s", err),
		}
	}

	logger.V(4).Info("Prepared devices for allocated claims", "devices", klog.Format(prepared))
	return &drapbv1.NodePrepareResourceResponse{CDIDevices: prepared}
}

func (d *driver) NodeUnprepareResources(ctx context.Context, req *drapbv1.NodeUnprepareResourcesRequest) (*drapbv1.NodeUnprepareResourcesResponse, error) {
	logger := klog.FromContext(ctx)
	logger.Info("NodeUnPrepareResource is called", "nclaims", len(req.Claims))
	unpreparedResources := &drapbv1.NodeUnprepareResourcesResponse{Claims: map[string]*drapbv1.NodeUnprepareResourceResponse{}}

	for _, claim := range req.Claims {
		unpreparedResources.Claims[claim.Uid] = d.nodeUnprepareResource(ctx, claim)
	}

	return unpreparedResources, nil
}

func (d *driver) nodeUnprepareResource(ctx context.Context, claim *drapbv1.Claim) *drapbv1.NodeUnprepareResourceResponse {
	d.Lock()
	defer d.Unlock()

	logger := klog.FromContext(ctx)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("NodeUnprepareResource is called")

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.V(4).Info("Getting NodeAllocationState and hold it onto driver struct for unpreparing resource")
		if err := d.nasclient.Get(); err != nil {
			return err
		}
		logger.V(4).Info("Unpreparing devices for claim")
		if err := d.state.Unprepare(ctx, claim.Uid); err != nil {
			return fmt.Errorf("error unpreparing devices for claim %q: %w", claim.Uid, err)
		}

		if err := d.nasclient.Update(d.state.GetUpdatedSpec(&d.nascrd.Spec)); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return &drapbv1.NodeUnprepareResourceResponse{
			Error: fmt.Sprintf("error unpreparing resource: %s", err),
		}
	}

	logger.V(4).Info("Unprepared devices for unallocated resource claim")
	return &drapbv1.NodeUnprepareResourceResponse{}
}
