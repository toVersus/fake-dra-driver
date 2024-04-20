package main

import (
	"context"
	"math/rand"
	"os"

	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

const (
	perNodeFakeDevices = 8
	fakeModelUltra10   = "ULTRA_10"
	fakeModelUltra100  = "ULTRA_100"
	fakeDevicePrefix   = "FAKE-"
)

var (
	fakeDevicePrefixLength = len(fakeDevicePrefix)
)

func enumerateSplittedFakeDevices(ctx context.Context, parentUUID string, model string, split int) []*FakeInfo {
	logger := klog.FromContext(ctx).WithValues("parentUID", parentUUID)
	uuids := generateUUIDs(parentUUID, split)

	splittedDevices := []*FakeInfo{}
	for _, uuid := range uuids {
		deviceInfo := &FakeInfo{
			uuid:   uuid,
			model:  model,
			parent: parentUUID,
		}
		logger.Info("Enumerating split fake devices", "deviceUID", uuid)
		splittedDevices = append(splittedDevices, deviceInfo)
	}

	return splittedDevices
}

func enumerateAllPossibleDevices(ctx context.Context) (AllocatableDevices, error) {
	logger := klog.FromContext(ctx)
	seed := os.Getenv("NODE_NAME")
	uuids := generateUUIDs(seed, perNodeFakeDevices)
	fakeModel := generateModel(seed)

	allDevices := make(AllocatableDevices)
	for _, uuid := range uuids {
		deviceInfo := &AllocatableDeviceInfo{
			FakeInfo: &FakeInfo{
				uuid:  uuid,
				model: fakeModel,
			},
		}
		logger.Info("Enumerating fake devices", "deviceUID", uuid)
		allDevices[uuid] = deviceInfo
	}
	return allDevices, nil
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

func generateModel(seed string) string {
	rand := rand.New(rand.NewSource(hash(seed)))
	// Randomly select a model from fakeModel10 and fakeModel100
	var fakeModel string
	if rand.Intn(2) == 0 {
		fakeModel = fakeModelUltra10
	} else {
		fakeModel = fakeModelUltra100
	}
	return fakeModel
}

func hash(s string) int64 {
	h := int64(0)
	for _, c := range s {
		h = 31*h + int64(c)
	}
	return h
}
