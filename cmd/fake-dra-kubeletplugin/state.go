package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	resourceapi "k8s.io/api/resource/v1alpha2"
	"k8s.io/klog/v2"
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
		return nascrd.FakeDeviceType
	}
	return nascrd.UnknownDeviceType
}

func (d PreparedDevices) Len() int {
	if d.Fake != nil {
		return len(d.Fake.Devices)
	}
	return 0
}

func (d *PreparedDevices) UUIDs() []string {
	var deviceUUIDs []string
	switch d.Type() {
	case nascrd.FakeDeviceType:
		for _, device := range d.Fake.Devices {
			deviceUUIDs = append(deviceUUIDs, device.uuid)
		}
	}
	return deviceUUIDs
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

	// NodeAllocationState の CRD に記載されている PreparedDevices を読み込んで、
	// DeviceState に反映する
	if err := state.syncPreparedDevicesFromCRDSpec(&config.nascrd.Spec); err != nil {
		return nil, fmt.Errorf("unable to sync prepared devices from CRD: %w", err)
	}
	logger.Info("Prepared all devices from CRD spec")

	return state, nil
}

func (s *DeviceState) Prepare(ctx context.Context, claimUID string, allocation nascrd.AllocatedDevices) ([]string, error) {
	logger := klog.FromContext(ctx).WithValues(
		"deviceType", allocation.Type(),
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

	if allocation.Type() == nascrd.FakeDeviceType {
		logger.V(4).Info("Preparing fake devices")
		fakes, err := s.prepareFakes(ctx, allocation.Fake)
		if err != nil {
			return nil, fmt.Errorf("allocation failed: %w", err)
		}
		prepared.Fake = fakes
	}

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
	case nascrd.FakeDeviceType:
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

func (s *DeviceState) GetUpdatedSpec(inspec *nascrd.NodeAllocationStateSpec) *nascrd.NodeAllocationStateSpec {
	s.Lock()
	defer s.Unlock()

	outspec := inspec.DeepCopy()
	s.syncAllocatableDevicesToCRDSpec(outspec)
	s.syncPreparedDevicesToCRDSpec(outspec)
	return outspec
}

func (s *DeviceState) prepareFakes(ctx context.Context, allocated *nascrd.AllocatedFakes) (*PreparedFakes, error) {
	logger := klog.FromContext(ctx)
	prepared := &PreparedFakes{}

	for _, device := range allocated.Devices {
		if _, ok := s.allocatable[device.UUID]; !ok {
			return nil, fmt.Errorf("requested Fake does not exist: %q", device.UUID)
		}

		fakeInfo := s.allocatable[device.UUID].FakeInfo

		if device.Split > 0 {
			logger.Info("Detected split device. Preparing new device", "parentUID", device.UUID, "split", device.Split)
			splittedFakeInfo := enumerateSplittedFakeDevices(ctx, device.UUID, fakeInfo.model, device.Split)
			prepared.Devices = append(prepared.Devices, splittedFakeInfo...)
		} else {
			logger.Info("Preparing fake device", "deviceUID", device.UUID)
			prepared.Devices = append(prepared.Devices, fakeInfo)
		}
	}
	return prepared, nil
}

func (s *DeviceState) unprepareFakes(_ string, _ *PreparedDevices) error {
	return nil
}

func (s *DeviceState) syncAllocatableDevicesToCRDSpec(spec *nascrd.NodeAllocationStateSpec) {
	fakes := make(map[string]nascrd.AllocatableDevice)
	for _, device := range s.allocatable {
		fakes[device.uuid] = nascrd.AllocatableDevice{
			Fake: &nascrd.AllocatableFake{
				UUID: device.uuid,
				Name: device.model,
			},
		}
	}

	var allocatable []nascrd.AllocatableDevice
	for _, device := range fakes {
		allocatable = append(allocatable, device)
	}

	spec.AllocatableDevice = allocatable
}

func (s *DeviceState) syncPreparedDevicesFromCRDSpec(spec *nascrd.NodeAllocationStateSpec) error {
	allocatable := s.allocatable

	prepared := make(PreparedClaims)
	for claim, devices := range spec.PreparedDevices {
		switch devices.Type() {
		case nascrd.FakeDeviceType:
			fakeDevices := &PreparedFakes{}
			prepared[claim] = &PreparedDevices{fakeDevices}
			for _, d := range devices.Fake.Devices {
				prepared[claim].Fake.Devices = append(prepared[claim].Fake.Devices, allocatable[d.UUID].FakeInfo)
			}
		}
	}

	s.prepared = prepared
	return nil
}

func (s *DeviceState) syncPreparedDevicesToCRDSpec(spec *nascrd.NodeAllocationStateSpec) {
	outcas := make(map[string]nascrd.PreparedDevices)
	for claim, devices := range s.prepared {
		var prepared nascrd.PreparedDevices
		switch devices.Type() {
		case nascrd.FakeDeviceType:
			prepared.Fake = &nascrd.PreparedFakes{}
			for _, device := range devices.Fake.Devices {
				outdevice := nascrd.PreparedFake{
					UUID: device.uuid,
				}
				if len(device.parent) != 0 {
					outdevice.ParentUUID = device.parent
				}
				prepared.Fake.Devices = append(prepared.Fake.Devices, outdevice)
			}
		}
		outcas[claim] = prepared
	}
	spec.PreparedDevices = outcas
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
