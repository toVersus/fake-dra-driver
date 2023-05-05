package v1alpha1

import (
	nascrdv1alpha1 "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
)

const (
	GroupName = "fake.resource.3-shake.com"
	Version   = "v1alpha1"

	FakeClaimParametersKind = "FakeClaimParameters"
)

func DefaultDeviceClassParametersSpec() *DeviceClassParametersSpec {
	return &DeviceClassParametersSpec{
		DeviceSelector: []DeviceSelector{
			{
				Type: nascrdv1alpha1.FakeDeviceType,
				Name: "*",
			},
		},
	}
}

func DefaultFakeClaimParametersSpec() *FakeClaimParametersSpec {
	return &FakeClaimParametersSpec{
		Count: 1,
	}
}
