package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	resourceapi "k8s.io/api/resource/v1alpha2"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1alpha3"

	fakecrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
)

var _ drapbv1.NodeServer = &driver{}

type driver struct {
	sync.Mutex
	doneCh chan struct{}

	state *DeviceState
}

func NewDriver(ctx context.Context, config *Config) (*driver, error) {
	logger := klog.FromContext(ctx)

	logger.V(4).Info("Generating mock Fake devices")
	state, err := NewDeviceState(ctx, config)
	if err != nil {
		return nil, err
	}

	return &driver{
		state: state,
	}, nil
}

func (d *driver) Shutdown(ctx context.Context) error {
	logger := klog.FromContext(ctx)
	logger.V(2).Info("Updating status of NodeAllocationState to NotReady before shutting down fake-dra-driver")
	defer close(d.doneCh)

	return nil
}

func (d *driver) NodeListAndWatchResources(req *drapbv1.NodeListAndWatchResourcesRequest, stream drapbv1.Node_NodeListAndWatchResourcesServer) error {
	resourceModel := d.state.getResourceModelFromAllocatableDevices()
	resp := &drapbv1.NodeListAndWatchResourcesResponse{
		Resources: []*resourceapi.ResourceModel{&resourceModel},
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
	ctx = klog.NewContext(ctx, logger)
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

	if len(claim.StructuredResourceHandle) == 0 {
		return &drapbv1.NodePrepareResourceResponse{
			Error: "No StructuredResourceHandle found in claim, please enable StructuredParameters in ResourceClass",
		}
	}

	logger.V(4).Info("[Structured Parameters] Preparing devices for claim")
	devices, split, err := d.prepareDevices(ctx, claim)
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("error allocating devices for claim %v: %s", claim.Uid, err),
		}
	}

	logger.V(4).Info("Preparing devices for claim")
	prepared, err = d.state.Prepare(ctx, claim.Uid, devices, split)
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("error preparing devices for claim %v: %s", claim.Uid, err),
		}
	}

	logger.V(4).Info("Prepared devices for allocated claims", "devices", klog.Format(prepared))
	return &drapbv1.NodePrepareResourceResponse{CDIDevices: prepared}
}

func (d *driver) isPrepared(ctx context.Context, claimUID string) (bool, []string, error) {
	logger := klog.FromContext(ctx)

	if prepared, exists := d.state.prepared[claimUID]; exists {
		claimedDevices := d.state.cdi.GetClaimDevices(claimUID, prepared)
		logger.V(4).Info("Claimed devices for claim", "claimUID", claimUID, "claimedDevices", claimedDevices)
		return true, claimedDevices, nil
	}
	logger.Info("Claim is not prepared", "claimUID", claimUID)
	return false, nil, nil
}

func (d *driver) prepareDevices(ctx context.Context, claim *drapbv1.Claim) ([]string, int, error) {
	logger := klog.FromContext(ctx)

	logger.V(4).Info("Getting vendor claim parameters", "claim", claim.Name)
	fakeClaimParams := fakecrd.FakeClaimParametersSpec{}
	logger.V(2).Info("Unmarshalling vendor request parameters", "raw", string(claim.StructuredResourceHandle[0].VendorClaimParameters.Raw))
	if err := json.Unmarshal(claim.StructuredResourceHandle[0].VendorClaimParameters.Raw, &fakeClaimParams); err != nil {
		return nil, 0, fmt.Errorf("error unmarshalling vendor request parameters: %w", err)
	}
	split := 0
	if fakeClaimParams.Split > 0 {
		logger.V(4).Info("Detected split device. Allocating splitted devices", "split", split)
		split = fakeClaimParams.Split
	}

	logger.V(4).Info("Allocating devices for claim", "claim", claim.Name)
	preparedDevices := make([]string, len(claim.StructuredResourceHandle[0].Results))
	for idx, r := range claim.StructuredResourceHandle[0].Results {
		name := r.AllocationResultModel.NamedResources.Name
		logger.V(4).Info("Allocate named resource", "name", name)
		fake := FakeInfo{
			uuid: fakeDevicePrefix + name[fakeDevicePrefixLength:],
		}
		preparedDevices[idx] = fake.uuid
	}

	return preparedDevices, split, nil
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

	logger.V(4).Info("Unpreparing devices for claim", "claimUID", claim.Uid)
	err = d.state.Unprepare(ctx, claim.Uid)
	if err != nil {
		return &drapbv1.NodeUnprepareResourceResponse{
			Error: fmt.Sprintf("error unpreparing devices for claim: %s", err),
		}

	}

	logger.V(4).Info("Unprepared devices for unallocated resource claim")
	return &drapbv1.NodeUnprepareResourceResponse{}
}

func (d *driver) isUnprepared(ctx context.Context, claimUID string) (bool, error) {
	logger := klog.FromContext(ctx)

	if _, exists := d.state.prepared[claimUID]; !exists {
		logger.V(4).Info("Claim is already unprepared", "claimUID", claimUID)
		return true, nil
	}
	logger.Info("Claim is prepared", "claimUID", claimUID)
	return false, nil
}
