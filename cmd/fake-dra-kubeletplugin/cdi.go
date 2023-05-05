package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	cdiapi "github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	cdispec "github.com/container-orchestrated-devices/container-device-interface/specs-go"
	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	"k8s.io/klog/v2"
)

const (
	cdiVendor = "k8s." + DriverName
	cdiClass  = "fake"
	cdiKind   = cdiVendor + "/" + cdiClass

	cdiCommonDeviceName = "common"
)

type CDIHandler struct {
	registry cdiapi.Registry
}

func NewCDIHandler(ctx context.Context, config *Config) (*CDIHandler, error) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Getting CDI registry", "dir", *config.flags.cdiRoot)
	registry := cdiapi.GetRegistry(
		cdiapi.WithSpecDirs(*config.flags.cdiRoot),
	)

	logger.V(4).Info("Rescanning all CDI Spec directories and updating the state of the registry cache")
	err := registry.Refresh()
	if err != nil {
		return nil, fmt.Errorf("unable to reflesh the CDI registry: %w", err)
	}

	handler := &CDIHandler{
		registry: registry,
	}

	logger.V(4).Info("Created new CDI handler")
	return handler, nil
}

func (cdi *CDIHandler) GetDevice(ctx context.Context, device string) *cdiapi.Device {
	logger := klog.FromContext(ctx).WithValues("device", device)
	logger.V(4).Info("Getting CDI device")
	return cdi.registry.DeviceDB().GetDevice(device)
}

func (cdi *CDIHandler) CreateCommonSpecFile(ctx context.Context) error {
	logger := klog.FromContext(ctx).WithValues(
		"cdiDevice", cdiCommonDeviceName,
	)
	spec := &cdispec.Spec{
		Kind: cdiKind,
		Devices: []cdispec.Device{
			{
				Name: cdiCommonDeviceName,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("FAKE_NODE_NAME=%s", os.Getenv("NODE_NAME")),
						fmt.Sprintf("DRA_RESOURCE_DRIVER_NAME=%s", DriverName),
					},
				},
			},
		},
	}
	logger.V(4).Info("Creating common CDI spec file",
		"env", klog.KObjSlice(spec.Devices[0].ContainerEdits.Env))

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required common CDI spec version: %w", err)
	}
	logger.V(4).Info("Minimum required common CDI spec version", "cdiSpecVersion", minVersion)
	spec.Version = minVersion

	specName, err := cdiapi.GenerateNameForTransientSpec(spec, cdiCommonDeviceName)
	if err != nil {
		return fmt.Errorf("failed to generate Spec name for common CDI: %w", err)
	}
	logger.V(4).Info("Writing common CDI spec file", "cdiSpecName", specName)
	return cdi.registry.SpecDB().WriteSpec(spec, specName)
}

func (cdi *CDIHandler) CreateClaimSpecFile(ctx context.Context, claimUID string, devices *PreparedDevices) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdiClass, claimUID)
	logger := klog.FromContext(ctx).WithValues(
		"cdiSpecName", specName,
	)
	spec := &cdispec.Spec{
		Kind:    cdiKind,
		Devices: []cdispec.Device{},
	}

	fakeIndex := 0
	switch devices.Type() {
	case nascrd.FakeDeviceType:
		for _, device := range devices.Fake.Devices {
			cdiDevice := cdispec.Device{
				Name: device.uuid,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("FAKE_DEVICE_%d=%s", fakeIndex, device.uuid),
					},
				},
			}
			logger.V(4).Info("Creating claimed CDI spec file",
				"cdiDeviceName", cdiDevice.Name, "env", strings.Join(cdiDevice.ContainerEdits.Env, ","))
			spec.Devices = append(spec.Devices, cdiDevice)
			fakeIndex++
		}
	}

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %w", err)
	}
	logger.V(4).Info("Minimum required claimed CDI spec version", "cdiSpecVersion", minVersion)
	spec.Version = minVersion

	logger.V(4).Info("Writing claimed CDI spec file")
	return cdi.registry.SpecDB().WriteSpec(spec, specName)
}

func (cdi *CDIHandler) DeleteClaimSpecFile(claimUID string) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdiClass, claimUID)
	return cdi.registry.SpecDB().RemoveSpec(specName)
}

func (cdi *CDIHandler) GetClaimDevices(claimUID string, devices *PreparedDevices) []string {
	cdiDevices := []string{
		cdiapi.QualifiedName(cdiVendor, cdiClass, cdiCommonDeviceName),
	}

	switch devices.Type() {
	case nascrd.FakeDeviceType:
		for _, device := range devices.Fake.Devices {
			cdiDevice := cdiapi.QualifiedName(cdiVendor, cdiClass, device.uuid)
			cdiDevices = append(cdiDevices, cdiDevice)
		}
	}

	return cdiDevices
}
