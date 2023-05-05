package main

import (
	"context"
	"fmt"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	fakecrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1alpha2"
	"k8s.io/dynamic-resource-allocation/controller"
	"k8s.io/klog/v2"
)

type fakedriver struct {
	PendingAllocatedClaims *PerNodeAllocatedClaim
}

func NewFakeDriver() *fakedriver {
	return &fakedriver{
		PendingAllocatedClaims: NewPerNodeAllocatedClaims(),
	}
}

func (d *fakedriver) ValidateClaimParameters(claimParams *fakecrd.FakeClaimParametersSpec) error {
	if claimParams.Count < 1 {
		return fmt.Errorf("invalid number of Fakes requested: count=%d", claimParams.Count)
	}
	if claimParams.Split < 1 {
		return fmt.Errorf("invalid number of virtual Fakes requested: split=%d", claimParams.Count)
	}

	return nil
}

func (d *fakedriver) Allocate(ctx context.Context, crd *nascrd.NodeAllocationState, claim *resourcev1.ResourceClaim, claimParams *fakecrd.FakeClaimParametersSpec,
	class *resourcev1.ResourceClass, classParams *fakecrd.DeviceClassParametersSpec, selectedNode string) (OnSuccesCallback, error) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Allocating Fake resource")

	claimUID := string(claim.UID)
	// 候補のノードに割り当て待ちのリソース要求がないか確認。
	// UnsuitableNodeForFakeClaim で設定した割り当て待ちのリソース要求があるはずなので、
	// なければエラーを返す。
	if !d.PendingAllocatedClaims.Exists(claimUID, selectedNode) {
		return nil, fmt.Errorf("no pending claim allocations %q on node %q", claimUID, selectedNode)
	}

	// NodeAllocateState の割り当て済みのリソース要求を設定する
	crd.Spec.AllocatedClaims[claimUID] = d.PendingAllocatedClaims.Get(claimUID, selectedNode)
	// リソースの割り当てに成功した場合に、割り当て待ちのリソース要求の一覧から削除するコールバックを設定する
	onSuccess := func() {
		d.PendingAllocatedClaims.Remove(claimUID)
	}

	return onSuccess, nil
}

func (d *fakedriver) Deallocate(ctx context.Context, crd *nascrd.NodeAllocationState, claim *resourcev1.ResourceClaim) error {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Deallocating Fake resource")
	d.PendingAllocatedClaims.Remove(string(claim.UID))
	return nil
}

// UnsuitableNodeForFakeClaim checks to see if the potential node is inappropriate for a Fake claim.
func (d *fakedriver) UnsuitableNodeForFakeClaim(ctx context.Context, crd *nascrd.NodeAllocationState, pod *corev1.Pod, fakecas []*controller.ClaimAllocation, allcas []*controller.ClaimAllocation, potentialNode string) error {
	// Fake リソースを割り当てる前に、リソース割り当ての観点でノードの候補が適切かを確認する
	logger := klog.FromContext(ctx).WithValues(
		"nodeAllocationState", klog.KObj(crd),
	)

	logger.V(4).Info("Syncing stalled pending and allocated claims on potential node before identifying unsuitable node")
	d.PendingAllocatedClaims.VisitNode(potentialNode, func(claimUID string, allocation nascrd.AllocatedDevices) {
		if _, ok := crd.Spec.AllocatedClaims[claimUID]; ok {
			logger.V(4).Info("Fake claim already allocated, removed from pending allocated claims")
			d.PendingAllocatedClaims.Remove(claimUID)
		} else {
			logger.V(4).Info("Fake claim not yet allocated, added to allocated claims")
			crd.Spec.AllocatedClaims[claimUID] = allocation
		}
	})

	// Fake デバイスを割り当てつつ、割り当て済みのデバイス一覧を取得する.
	allocated := d.allocateFakes(crd, pod, fakecas, allcas, potentialNode)
	// Fake リソースの要求を見ていって、割り当て済みデバイス一覧
	for _, ca := range fakecas {
		claimUID := string(ca.Claim.UID)
		claimParams := ca.ClaimParameters.(*fakecrd.FakeClaimParametersSpec)

		// 指定された Fake リソース数を満たしていない場合は、そのノードは Fake デバイスを割り当てることができないので、
		// UnsuitableNodes にノードの情報を詰め込んで、Pod がスケジュールされるノードの候補から除外する.
		if claimParams.Count != len(allocated[claimUID]) {
			logger.V(2).Info("Not enough Fake devices available on node, return potential node as unsuitable node")
			for _, ca := range allcas {
				ca.UnsuitableNodes = append(ca.UnsuitableNodes, potentialNode)
			}
			return nil
		}

		// 先ほど取得した Fake リソースの割り当て済みデバイス一覧を変数に詰め直す
		var devices []nascrd.AllocatedFake
		split := claimParams.Split
		for _, fake := range allocated[claimUID] {
			device := nascrd.AllocatedFake{
				UUID: fake,
			}
			if split > 0 {
				logger.V(2).Info("Passing split paramter to allocated devices", "split", claimParams.Split)
				device.Split = split
			}
			devices = append(devices, device)
		}
		allocatedDevices := nascrd.AllocatedDevices{
			Fake: &nascrd.AllocatedFakes{
				Devices: devices,
			},
		}

		klog.V(4).Info("Saving pending allocations")
		// 遅延割り当てを管理する PendingAllocatedClaims に Fake リソースの割り当て済み要求として登録し、
		// Allocate API が呼び出されたときに
		d.PendingAllocatedClaims.Set(claimUID, potentialNode, allocatedDevices)
	}

	return nil
}

// allocateFakes allocates Fake devices on the node if not allocated yet,
// and returns the allocated Fake devices.
func (d *fakedriver) allocateFakes(crd *nascrd.NodeAllocationState, pod *corev1.Pod, fakecas []*controller.ClaimAllocation, allcas []*controller.ClaimAllocation, node string) map[string][]string {
	// ノード上で割り当て可能な Fake デバイスを取得して変数に詰め込む
	available := make(map[string]*nascrd.AllocatableFake)
	for _, device := range crd.Spec.AllocatableDevice {
		if device.Type() == nascrd.FakeDeviceType {
			available[device.Fake.UUID] = device.Fake
		}
	}

	// ノード上で割り当て可能な Fake デバイスから, 既に割り当てられているデバイスを削除する.
	for _, allocation := range crd.Spec.AllocatedClaims {
		if allocation.Type() == nascrd.FakeDeviceType {
			for _, device := range allocation.Fake.Devices {
				delete(available, device.UUID)
			}
		}
	}

	allocated := make(map[string][]string)
	// Fake リソースの必要な ResourceClaim の一覧を見ていって、割り当て済みのリソース一覧を作成する.
	for _, ca := range fakecas {
		claimUID := string(ca.Claim.UID)
		// ResourceClaim の UID で既に割り当て済みのリソースがあれば,
		// 紐付いている Fake リソースのデバイス情報を割り当て済みのリソース一覧に追加.
		if _, ok := crd.Spec.AllocatedClaims[claimUID]; ok {
			devices := crd.Spec.AllocatedClaims[claimUID].Fake.Devices
			for _, device := range devices {
				allocated[claimUID] = append(allocated[claimUID], device.UUID)
			}
			continue
		}

		claimParams := ca.ClaimParameters.(*fakecrd.FakeClaimParametersSpec)
		var devices []string
		// ResourceClaim の UID に対して割り当て済みのリソースがなければ,
		// ResourceClaim のパラメータから必要なリソース数を取得して,
		// 割り当て可能なリソースから**実際にリソースを割り当てて**,
		// Fake デバイスの情報を割り当て済みのリソース一覧に追加.
		//
		// FakeClaimParameters には split も指定できるが,
		// split はあくまで count で作成したデバイスの子デバイスという扱いかつ,
		// split の数を考慮して Pod をスケジュールする必要はない (親デバイスさえ確保できれば問題ない) ので,
		// ここでは考慮しない.
		for i := 0; i < claimParams.Count; i++ {
			for _, device := range available {
				devices = append(devices, device.UUID)
				delete(available, device.UUID)
				break
			}
		}
		allocated[claimUID] = devices
	}

	return allocated
}
