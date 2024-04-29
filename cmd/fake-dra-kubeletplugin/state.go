package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	resourceapi "k8s.io/api/resource/v1alpha2"
	"k8s.io/klog/v2"

	fakev1alpha1 "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
)

type AllocatableDevices map[string]*AllocatableDeviceInfo
type PreparedClaims map[string]*PreparedDevices

type FakeInfo struct {
	uuid   string
	model  string
	parent string
}

type PreparedFakes struct {
	Devices []*FakeInfo
}

type PreparedDevices struct {
	Fake *PreparedFakes
}

func (d *PreparedDevices) Type() string {
	if d.Fake != nil {
		return fakev1alpha1.FakeDeviceType
	}
	return fakev1alpha1.UnknownDeviceType
}

type AllocatableDeviceInfo struct {
	*FakeInfo
}

type DeviceState struct {
	sync.Mutex
	cdi         *CDIHandler
	allocatable AllocatableDevices
	prepared    PreparedClaims
}

func NewDeviceState(ctx context.Context, config *Config) (*DeviceState, error) {
	logger := klog.FromContext(ctx)
	logger.V(2).Info("Enumerating all available devices")
	allocatable, err := enumerateAllPossibleDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("error enumerating all possible devices: %w", err)
	}

	cdi, err := NewCDIHandler(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI handler: %w", err)
	}

	logger.V(4).Info("Creating CDI spec file for common device")
	if err = cdi.CreateCommonSpecFile(ctx); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for common edits: %w", err)
	}

	state := &DeviceState{
		cdi:         cdi,
		allocatable: allocatable,
		prepared:    make(PreparedClaims),
	}

	return state, nil
}

func (s *DeviceState) Prepare(ctx context.Context, claimUID string, devices []string, split int) ([]string, error) {
	logger := klog.FromContext(ctx).WithValues(
		"resourceClaimUID", claimUID,
	)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("Prepare CDI spec file for claim")

	s.Lock()
	defer s.Unlock()

	if s.prepared[claimUID] != nil {
		logger.V(2).Info("Returning already prepared devices for claim")
		return s.cdi.GetClaimDevices(claimUID, s.prepared[claimUID]), nil
	}

	prepared := &PreparedDevices{}

	logger.V(4).Info("Preparing fake devices")
	fakes, err := s.prepareFakes(ctx, claimUID, devices, split)
	if err != nil {
		return nil, fmt.Errorf("allocation failed: %w", err)
	}
	prepared.Fake = fakes

	logger.V(4).Info("Creating CDI spec file for claim")
	if err := s.cdi.CreateClaimSpecFile(ctx, claimUID, prepared); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %w", err)
	}
	s.prepared[claimUID] = prepared
	logger.V(4).Info("Getting list of prepared CDI devices")
	return s.cdi.GetClaimDevices(claimUID, s.prepared[claimUID]), nil
}

func (s *DeviceState) Unprepare(ctx context.Context, claimUID string) error {
	logger := klog.FromContext(ctx).WithValues("resourceClaimUID", claimUID)
	logger.V(4).Info("Unprepare CDI spec file for claim")

	s.Lock()
	defer s.Unlock()

	if s.prepared[claimUID] == nil {
		return nil
	}

	switch s.prepared[claimUID].Type() {
	case fakev1alpha1.FakeDeviceType:
		klog.V(4).Info("Unpreparing fake devices")
		if err := s.unprepareFakes(claimUID, s.prepared[claimUID]); err != nil {
			return fmt.Errorf("unprepare failed: %w", err)
		}
	}

	logger.V(4).Info("Delete CDI spec file for claim")
	if err := s.cdi.DeleteClaimSpecFile(claimUID); err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %w", err)
	}

	delete(s.prepared, claimUID)
	return nil
}

func (s *DeviceState) prepareFakes(ctx context.Context, claimUID string, devices []string, split int) (*PreparedFakes, error) {
	logger := klog.FromContext(ctx)
	prepared := &PreparedFakes{}

	for _, uuid := range devices {
		fakeInfo := s.allocatable[uuid].FakeInfo

		if _, ok := s.allocatable[uuid]; !ok {
			return nil, fmt.Errorf("requested Fake does not exist: %q", uuid)
		}

		if split > 0 {
			logger.Info("Detected split device. Preparing new device", "parentUID", uuid, "split", split)
			splittedFakeInfo := enumerateSplittedFakeDevices(ctx, uuid, fakeInfo.model, split)
			prepared.Devices = append(prepared.Devices, splittedFakeInfo...)
		} else {
			logger.Info("Preparing fake device", "deviceUID", uuid)
			prepared.Devices = append(prepared.Devices, fakeInfo)
		}
	}
	return prepared, nil
}

func (s *DeviceState) unprepareFakes(claimUID string, devices *PreparedDevices) error {
	return nil
}

func (s *DeviceState) getResourceModelFromAllocatableDevices() resourceapi.ResourceModel {
	var instances []resourceapi.NamedResourcesInstance
	for _, device := range s.allocatable {
		instance := resourceapi.NamedResourcesInstance{
			Name: strings.ToLower(device.uuid),
			Attributes: []resourceapi.NamedResourcesAttribute{
				{
					Name: "uuid",
					NamedResourcesAttributeValue: resourceapi.NamedResourcesAttributeValue{
						StringValue: &device.uuid,
					},
				},
				{
					Name: "model",
					NamedResourcesAttributeValue: resourceapi.NamedResourcesAttributeValue{
						StringValue: &device.model,
					},
				},
			},
		}
		instances = append(instances, instance)
	}

	return resourceapi.ResourceModel{
		NamedResources: &resourceapi.NamedResourcesResources{Instances: instances},
	}
}
