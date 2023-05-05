package main

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	drav1alpha2 "k8s.io/kubelet/pkg/apis/dra/v1alpha2"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	nasclient "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1/client"
)

var _ drav1alpha2.NodeServer = &driver{}

type driver struct {
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

func (d *driver) NodePrepareResource(ctx context.Context, req *drav1alpha2.NodePrepareResourceRequest) (*drav1alpha2.NodePrepareResourceResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"resourceClaim", klog.KRef(req.Namespace, req.ClaimName),
		"resourceClaimUID", req.ClaimUid,
		"nodeAllocationState", klog.KObj(d.nascrd),
	)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("NodePrepareResource is called")

	var prepared []string
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.V(4).Info("Getting NodeAllocationState and hold it onto driver struct for preparing resource")
		err := d.nasclient.Get()
		if err != nil {
			return err
		}
		logger.V(4).Info("Preparing devices for claim")
		prepared, err = d.state.Prepare(ctx, req.ClaimUid, d.nascrd.Spec.AllocatedClaims[req.ClaimUid])
		if err != nil {
			return fmt.Errorf("error preparing devices for claim %q: %w", req.ClaimUid, err)
		}

		logger.V(4).Info("Updating spec of NodeAllocationState and add prepared devices to PreparedDevices field")
		if err := d.nasclient.Update(d.state.GetUpdatedSpec(&d.nascrd.Spec)); err != nil {
			if nestedErr := d.state.Unprepare(ctx, req.ClaimUid); nestedErr != nil {
				logger.Error(errors.Join(err, nestedErr), "Error unpreparing resource after claim Update() failed")
			} else {
				logger.Error(err, "Error updating NodeAllocationState status to NotReady, so unpreparing earlier prepared resource")
			}
			return err
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retry preparing resource: %w", err)
	}

	logger.V(4).Info("Prepared devices for allocated claims", "devices", klog.Format(prepared))
	return &drav1alpha2.NodePrepareResourceResponse{CdiDevices: prepared}, nil
}

func (d *driver) NodeUnprepareResource(ctx context.Context, req *drav1alpha2.NodeUnprepareResourceRequest) (*drav1alpha2.NodeUnprepareResourceResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"resourceClaim", klog.KRef(req.Namespace, req.ClaimName),
		"resourceClaimUID", req.ClaimUid,
		"nodeAllocationState", klog.KObj(d.nascrd),
	)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("NodeUnprepareResource is called")

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.V(4).Info("Getting NodeAllocationState and hold it onto driver struct for unpreparing resource")
		if err := d.nasclient.Get(); err != nil {
			return err
		}
		logger.V(4).Info("Unpreparing devices for claim")
		if err := d.state.Unprepare(ctx, req.ClaimUid); err != nil {
			return fmt.Errorf("error unpreparing devices for claim %q: %w", req.ClaimUid, err)
		}

		if err := d.nasclient.Update(d.state.GetUpdatedSpec(&d.nascrd.Spec)); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error unpreparing resource: %w", err)
	}

	logger.V(4).Info("Unprepared devices for unallocated resource claim")
	return &drav1alpha2.NodeUnprepareResourceResponse{}, nil
}
