package v1alpha1

const (
	GroupName = "fake.resource.3-shake.com"
	Version   = "v1alpha1"

	FakeClaimParametersKind = "FakeClaimParameters"
)

func DefaultDeviceClassParametersSpec() *DeviceClassParametersSpec {
	return &DeviceClassParametersSpec{
		DeviceSelector: []DeviceSelector{
			{
				Type: FakeDeviceType,
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
