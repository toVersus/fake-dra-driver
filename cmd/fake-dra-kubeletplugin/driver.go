package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	resourceapi "k8s.io/api/resource/v1alpha2"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1alpha3"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	nasclient "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1/client"
	fakecrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
)

var _ drapbv1.NodeServer = &driver{}

type driver struct {
	sync.Mutex
	doneCh    chan struct{}
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
	defer close(d.doneCh)
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
	model := d.state.getResourceModelFromAllocatableDevices()
	resp := &drapbv1.NodeListAndWatchResourcesResponse{
		Resources: []*resourceapi.ResourceModel{&model},
	}

	if err := stream.Send(resp); err != nil {
		return err
	}

	// Keep the stream open until the driver is shutdown
	<-d.doneCh

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
	logger.V(4).Info("NodePrepareResource is called")
	d.Lock()
	defer d.Unlock()

	logger.V(4).Info("Getting NodeAllocationState and hold it onto driver struct for preparing resource")
	isPrepared, prepared, err := d.isPrepared(ctx, claim.Uid)
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("error checking if claim is already prepared: %v", err),
		}
	}
	if isPrepared {
		logger.Info("Returning cached devices for claim", "claimUID", claim.Uid, "prepared", prepared)
		return &drapbv1.NodePrepareResourceResponse{CDIDevices: prepared}
	}

	if len(claim.StructuredResourceHandle) > 0 {
		logger.V(4).Info("Allocating devices for claim")
		err = d.allocateDevices(ctx, claim)
		if err != nil {
			return &drapbv1.NodePrepareResourceResponse{
				Error: fmt.Sprintf("error allocating devices for claim %v: %s", claim.Uid, err),
			}
		}
	} else {
		logger.V(4).Info("No StructuredResourceHandle found in claim, skipping allocation of devices")
	}

	logger.V(4).Info("Preparing devices for claim")
	prepared, err = d.state.Prepare(ctx, claim.Uid, d.nascrd.Spec.AllocatedClaims[claim.Uid])
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("error preparing devices for claim %v: %s", claim.Uid, err),
		}
	}

	logger.V(4).Info("Updating spec of NodeAllocationState and add prepared devices to PreparedDevices field")
	if err := d.nasclient.Update(d.state.GetUpdatedSpec(&d.nascrd.Spec)); err != nil {
		if nestedErr := d.state.Unprepare(ctx, claim.Uid); nestedErr != nil {
			logger.Error(errors.Join(err, nestedErr), "Error unpreparing resource after claim Update() failed")
		} else {
			logger.Error(err, "Error updating NodeAllocationState status to NotReady, so unpreparing earlier prepared resource")
		}
		return &drapbv1.NodePrepareResourceResponse{
			Error: err.Error(),
		}
	}

	logger.V(4).Info("Prepared devices for allocated claims", "devices", klog.Format(prepared))
	return &drapbv1.NodePrepareResourceResponse{CDIDevices: prepared}
}

func (d *driver) NodeUnprepareResources(ctx context.Context, req *drapbv1.NodeUnprepareResourcesRequest) (*drapbv1.NodeUnprepareResourcesResponse, error) {
	klog.Infof("NodeUnPrepareResource is called: number of claims: %d", len(req.Claims))
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

	isUnprepared, err := d.isUnprepared(ctx, claim.Uid)
	if err != nil {
		return &drapbv1.NodeUnprepareResourceResponse{
			Error: fmt.Sprintf("error checking if claim is already unprepared: %v", err),
		}
	}

	if isUnprepared {
		logger.Info("Already unprepared, nothing to do for claim", "claimUID", claim.Uid)
		return &drapbv1.NodeUnprepareResourceResponse{}
	}

	if len(claim.StructuredResourceHandle) > 0 {
		err = d.deallocateDevices(ctx, claim)
		if err != nil {
			return &drapbv1.NodeUnprepareResourceResponse{
				Error: fmt.Sprintf("error deallocating devices for claim %v: %v", claim.Uid, err),
			}
		}
	}

	logger.V(4).Info("Unpreparing devices for claim", "claimUID", claim.Uid)
	err = d.state.Unprepare(ctx, claim.Uid)
	if err != nil {
		return &drapbv1.NodeUnprepareResourceResponse{
			Error: fmt.Sprintf("error unpreparing devices for claim %v: %v", claim.Uid, err),
		}
	}

	return &drapbv1.NodeUnprepareResourceResponse{}
}

func (d *driver) isPrepared(ctx context.Context, claimUID string) (bool, []string, error) {
	logger := klog.FromContext(ctx)

	err := d.nasclient.Get()
	if err != nil {
		return false, nil, err
	}
	if prepared, exists := d.state.prepared[claimUID]; exists {
		claimedDevices := d.state.cdi.GetClaimDevices(claimUID, prepared)
		logger.V(4).Info("Claimed devices for claim", "claimUID", claimUID, "claimedDevices", claimedDevices)
		return true, claimedDevices, nil
	}
	logger.Info("Claim is not prepared", "claimUID", claimUID)
	return false, nil, nil
}

func (d *driver) isUnprepared(ctx context.Context, claimUID string) (bool, error) {
	logger := klog.FromContext(ctx)

	err := d.nasclient.Get()
	if err != nil {
		return false, err
	}
	if _, exists := d.state.prepared[claimUID]; !exists {
		logger.V(4).Info("Claim is already unprepared", "claimUID", claimUID)
		return true, nil
	}
	logger.Info("Claim is prepared", "claimUID", claimUID)
	return false, nil
}

func (d *driver) allocateDevices(ctx context.Context, claim *drapbv1.Claim) error {
	logger := klog.FromContext(ctx)

	logger.V(4).Info("Getting vendor claim parameters", "claim", claim.Name)
	fakeClaimParams := fakecrd.FakeClaimParametersSpec{}
	logger.V(2).Info("Unmarshalling vendor request parameters", "raw", string(claim.StructuredResourceHandle[0].VendorClaimParameters.Raw))
	if err := json.Unmarshal(claim.StructuredResourceHandle[0].VendorClaimParameters.Raw, &fakeClaimParams); err != nil {
		return fmt.Errorf("error unmarshalling vendor request parameters: %w", err)
	}
	split := fakeClaimParams.Split

	logger.V(4).Info("Allocating devices for claim", "claim", claim.Name)
	allocatedFakes := []nascrd.AllocatedFake{}
	for _, r := range claim.StructuredResourceHandle[0].Results {
		name := r.AllocationResultModel.NamedResources.Name
		logger.V(4).Info("Allocate named resource", "name", name)
		fake := nascrd.AllocatedFake{
			UUID: fakeDevicePrefix + name[fakeDevicePrefixLength:],
		}

		if split > 0 {
			logger.V(4).Info("Detected split device. Allocating splitted devices", "name", name, "split", split)
			fake.Split = split
		}

		allocatedFakes = append(allocatedFakes, fake)
	}
	allocated := nascrd.AllocatedDevices{
		Fake: &nascrd.AllocatedFakes{
			Devices: allocatedFakes,
		},
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.nasclient.Get()
		if err != nil {
			return err
		}

		if len(d.nascrd.Spec.AllocatedClaims) == 0 {
			d.nascrd.Spec.AllocatedClaims = make(map[string]nascrd.AllocatedDevices)
		}
		d.nascrd.Spec.AllocatedClaims[claim.Uid] = allocated

		err = d.nasclient.Update(&d.nascrd.Spec)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (d *driver) deallocateDevices(ctx context.Context, claim *drapbv1.Claim) error {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Deallocating devices for claim", "claim", claim)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.nasclient.Get()
		if err != nil {
			return err
		}

		delete(d.nascrd.Spec.AllocatedClaims, claim.Uid)

		err = d.nasclient.Update(&d.nascrd.Spec)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
