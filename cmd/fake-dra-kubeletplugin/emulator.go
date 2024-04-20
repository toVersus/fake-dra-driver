package main

import (
	"context"
	"math/rand"
	"os"

	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

const (
	fakeModel        = "LATEST-FAKE-MODEL"
	fakeDevicePrefix = "FAKE-"
)

var (
	fakeDevicePrefixLength = len(fakeDevicePrefix)
)

func enumerateSplittedFakeDevices(ctx context.Context, parentUUID string, split int) []*FakeInfo {
	logger := klog.FromContext(ctx).WithValues("parentUID", parentUUID)
	uuids := generateUUIDs(parentUUID, split)

	splittedDevices := []*FakeInfo{}
	for _, uuid := range uuids {
		deviceInfo := &FakeInfo{
			uuid:   uuid,
			model:  fakeModel,
			parent: parentUUID,
		}
		logger.Info("Enumerating split fake devices", "deviceUID", uuid)
		splittedDevices = append(splittedDevices, deviceInfo)
	}

	return splittedDevices
}

func enumerateAllPossibleDevices(ctx context.Context) (AllocatableDevices, error) {
	logger := klog.FromContext(ctx)
	numFakes := 8
	seed := os.Getenv("NODE_NAME")
	uuids := generateUUIDs(seed, numFakes)

	alldevices := make(AllocatableDevices)
	for _, uuid := range uuids {
		deviceInfo := &AllocatableDeviceInfo{
			FakeInfo: &FakeInfo{
				uuid:  uuid,
				model: fakeModel,
			},
		}
		logger.Info("Enumerating fake devices", "deviceUID", uuid)
		alldevices[uuid] = deviceInfo
	}
	return alldevices, nil
}

func generateUUIDs(seed string, count int) []string {
	rand := rand.New(rand.NewSource(hash(seed)))

	uuids := make([]string, count)
	for i := 0; i < count; i++ {
		charset := make([]byte, 16)
		rand.Read(charset)
		uuid, _ := uuid.FromBytes(charset)
		uuids[i] = fakeDevicePrefix + uuid.String()
	}
	return uuids
}

func hash(s string) int64 {
	h := int64(0)
	for _, c := range s {
		h = 31*h + int64(c)
	}
	return h
}
