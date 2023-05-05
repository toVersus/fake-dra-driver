package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/dynamic-resource-allocation/controller"
	"k8s.io/klog/v2"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	nasclient "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1/client"
	fakecrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
	clientset "github.com/toVersus/fake-dra-driver/pkg/3-shake.com/resource/clientset/versioned"
)

const (
	DriverName     = fakecrd.GroupName
	DriverAPIGroup = fakecrd.GroupName
)

type OnSuccesCallback func()

type driver struct {
	lock      *PerNodeMutex
	namespace string
	clientset clientset.Interface
	fake      *fakedriver
}

var _ controller.Driver = (*driver)(nil)

func NewDriver(config *Config) *driver {
	return &driver{
		lock:      NewPerNodeMutex(),
		namespace: config.namespace,
		clientset: config.clientset.shake,
		fake:      NewFakeDriver(),
	}
}

func (d *driver) GetClassParameters(ctx context.Context, class *resourcev1.ResourceClass) (interface{}, error) {
	logger := klog.FromContext(ctx).WithValues("resourceClass", klog.KObj(class))
	logger.V(4).Info("GetClassParameters gets called to retrieve the parameter object referenced by a ResourceClass")
	if class.ParametersRef == nil {
		logger.V(2).Info("No ParametersRef specified, using default parameters")
		return fakecrd.DefaultDeviceClassParametersSpec(), nil
	}
	if class.ParametersRef.APIGroup != DriverAPIGroup {
		return nil, fmt.Errorf("incorrect API group: %v", class.ParametersRef.APIGroup)
	}
	dc, err := d.clientset.FakeV1alpha1().DeviceClassParameters().Get(ctx, class.ParametersRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting DeviceClassParameters called '%v': %w", class.ParametersRef.Name, err)
	}
	return &dc.Spec, nil
}

func (d *driver) GetClaimParameters(ctx context.Context, claim *resourcev1.ResourceClaim, class *resourcev1.ResourceClass, classParameters interface{}) (interface{}, error) {
	logger := klog.FromContext(ctx).WithValues(
		"resourceClaim", klog.KObj(claim),
		"resourceClass", klog.KObj(class),
	)
	logger.V(4).Info("GetClaimParameters gets called to retrieve the parameter object referenced by a CRD based claim parameters")
	if claim.Spec.ParametersRef == nil {
		return fakecrd.DefaultFakeClaimParametersSpec(), nil
	}
	if claim.Spec.ParametersRef.APIGroup != DriverAPIGroup {
		return nil, fmt.Errorf("incorrect API group: %v", claim.Spec.ParametersRef.APIGroup)
	}

	if claim.Spec.ParametersRef.Kind == fakecrd.FakeClaimParametersKind {
		fc, err := d.clientset.FakeV1alpha1().FakeClaimParameters(claim.Namespace).Get(ctx, claim.Spec.ParametersRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting FakeClaim called %q in namespace %q: %w", claim.Spec.ParametersRef.Name, claim.Namespace, err)
		}
		logger.V(2).Info("Validating FakeClaim parameters", "fakeClaimParameters", klog.KObj(fc))
		err = d.fake.ValidateClaimParameters(&fc.Spec)
		if err != nil {
			return nil, fmt.Errorf("error validating FakeClaim called %q in namespace %q: %w", claim.Spec.ParametersRef.Name, claim.Namespace, err)
		}
		return &fc.Spec, nil
	}
	return nil, fmt.Errorf("unknown ResourceClaim.ParametersRef.Kind: %v", claim.Spec.ParametersRef.Kind)
}

func (d *driver) Allocate(ctx context.Context, claim *resourcev1.ResourceClaim, claimParameters interface{},
	class *resourcev1.ResourceClass, classParameters interface{}, selectedNode string) (*resourcev1.AllocationResult, error) {
	logger := klog.FromContext(ctx).WithValues(
		"resourceClaim", klog.KObj(claim),
		"resourceClass", klog.KObj(class),
		"selectedNode", selectedNode,
	)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("Allocate gets called when a ResourceClaim is ready to be allocated")

	// リソースの遅延割り当ての場合は、必ず selectedNode が設定される
	if selectedNode == "" {
		return nil, fmt.Errorf("TODO: immediate allocation not yet supported")
	}

	d.lock.Get(selectedNode).Lock()
	defer d.lock.Get(selectedNode).Unlock()

	crdconfig := &nascrd.NodeAllocationStateConfig{
		Name:      selectedNode,
		Namespace: d.namespace,
	}
	crd := nascrd.NewNodeAllocationState(crdconfig)
	client := nasclient.New(crd, d.clientset.NasV1alpha1())
	// 選ばれしノードの NodeAllocationState は存在するはずなので、存在しない場合はエラーを返す
	if err := client.Get(); err != nil {
		return nil, fmt.Errorf("error retrieving node specific Fake CRD: %w", err)
	}
	// 生成した NodeAllocationState の spec は空っぽなので必ず初期化されるけど、
	// 念のため nil チェック
	if crd.Spec.AllocatedClaims == nil {
		crd.Spec.AllocatedClaims = make(map[string]nascrd.AllocatedDevices)
	}
	// 生成した NodeAllocationState の spec は空っぽなのでこの条件にはマッチしないはず
	if _, ok := crd.Spec.AllocatedClaims[string(claim.UID)]; ok {
		logger.V(2).Info("Claim already allocated on node")
		return buildAllocationResult(ctx, selectedNode, true), nil
	}

	if crd.Status == nascrd.NodeAllocationStateStatusNotReady {
		return nil, fmt.Errorf("node %q is not ready: %v", selectedNode, crd.Status)
	}

	var onSuccess OnSuccesCallback
	var err error
	classParams := classParameters.(*fakecrd.DeviceClassParametersSpec)
	switch claimParams := claimParameters.(type) {
	case *fakecrd.FakeClaimParametersSpec:
		onSuccess, err = d.fake.Allocate(ctx, crd, claim, claimParams, class, classParams, selectedNode)
	default:
		err = fmt.Errorf("unknown ResourceClaim.ParametersRef.Kind: %s", claim.Spec.ParametersRef.Kind)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to allocate devices for selected node %q: %w", selectedNode, err)
	}

	// NodeAllocateState の割り当て済みのリソース要求の変更を反映する
	err = client.Update(&crd.Spec)
	if err != nil {
		return nil, fmt.Errorf("error updating NodeAllocationState CRD: %w", err)
	}

	logger.V(2).Info("Cleanup pending allocated claim for selected node")
	// NodeAllocateState の割り当て済みのリソース要求の更新に成功したらコールバックを実行する
	onSuccess()

	logger.V(2).Info("Allocated resource claim for selected node")
	return buildAllocationResult(ctx, selectedNode, true), nil
}

func (d *driver) Deallocate(ctx context.Context, claim *resourcev1.ResourceClaim) error {
	logger := klog.FromContext(ctx).WithValues(
		"resourceClaim", klog.KObj(claim),
	)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("Deallocation gets called when a ResourceClaim is ready to be freed")

	if claim.Status.Allocation == nil {
		logger.V(2).Info("Allocation not found in status of ResourceClaim. Skip deallocating resources")
		return nil
	}

	selectedNode := getSelectedNode(ctx, claim)
	if selectedNode == "" {
		logger.V(2).Info("ResourceClaim not allocated on any node")
		return nil
	}

	d.lock.Get(selectedNode).Lock()
	defer d.lock.Get(selectedNode).Unlock()

	crdconfig := &nascrd.NodeAllocationStateConfig{
		Name:      selectedNode,
		Namespace: d.namespace,
	}
	crd := nascrd.NewNodeAllocationState(crdconfig)

	client := nasclient.New(crd, d.clientset.NasV1alpha1())
	if err := client.Get(); err != nil {
		return fmt.Errorf("error retrieving node specific Fake GRD: %w", err)
	}

	if crd.Spec.AllocatedClaims == nil {
		return nil
	}

	if _, ok := crd.Spec.AllocatedClaims[string(claim.UID)]; !ok {
		return nil
	}

	devices := crd.Spec.AllocatedClaims[string(claim.UID)]
	var err error
	switch devices.Type() {
	case nascrd.FakeDeviceType:
		err = d.fake.Deallocate(ctx, crd, claim)
	default:
		err = fmt.Errorf("unknown AllocatedDevices.Type(): %v", devices.Type())
	}
	if err != nil {
		return fmt.Errorf("unable to deallocate devices %v: %w", devices, err)
	}

	delete(crd.Spec.AllocatedClaims, string(claim.UID))

	err = client.Update(&crd.Spec)
	if err != nil {
		return fmt.Errorf("error updating NodeAllocationState CRD: %w", err)
	}

	return nil
}

func (d *driver) UnsuitableNodes(ctx context.Context, pod *corev1.Pod, cas []*controller.ClaimAllocation, potentialNodes []string) error {
	logger := klog.FromContext(ctx).WithValues(
		"pod", klog.KObj(pod),
	)
	ctx = klog.NewContext(ctx, logger)
	logger.V(4).Info("UnsuitableNodes checks all pending claims with delayed allocation for a pod")

	for _, node := range potentialNodes {
		err := d.unsuitableNode(ctx, pod, cas, node)
		if err != nil {
			return fmt.Errorf("error processing node %q: %w", node, err)
		}
	}

	for _, ca := range cas {
		ca.UnsuitableNodes = unique(ca.UnsuitableNodes)
	}
	return nil
}

func buildAllocationResult(ctx context.Context, selectedNode string, shareable bool) *resourcev1.AllocationResult {
	logger := klog.FromContext(ctx)
	nodeSelector := &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{
				MatchFields: []corev1.NodeSelectorRequirement{
					{
						Key:      "metadata.name",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{selectedNode},
					},
				},
			},
		},
	}
	logger.V(4).Info("Building allocation result for selected node", "nodeSelector", klog.Format(nodeSelector))
	return &resourcev1.AllocationResult{
		AvailableOnNodes: nodeSelector,
		Shareable:        shareable,
	}
}

func (d *driver) unsuitableNode(ctx context.Context, pod *corev1.Pod, allcas []*controller.ClaimAllocation, potentialNode string) error {
	logger := klog.FromContext(ctx).WithValues(
		"potentialNode", potentialNode,
	)
	ctx = klog.NewContext(ctx, logger)

	// ノード単位でリソースを割り当てられないノードを見つけていくので、
	// 同一のノードに対して複数の処理が走らないように、ノード名でロックを取得
	d.lock.Get(potentialNode).Lock()
	defer d.lock.Get(potentialNode).Unlock()

	// 候補のノードの NodeAllocationState のカスタムリソースを取得したいので、
	// 必要な情報を埋めてインスタンスを作成
	crdconfig := &nascrd.NodeAllocationStateConfig{
		Name:      potentialNode,
		Namespace: d.namespace,
	}
	crd := nascrd.NewNodeAllocationState(crdconfig)

	client := nasclient.New(crd, d.clientset.NasV1alpha1())
	// 候補のノードのリソースの割り当て状況を見たいので、NodeAllocationState のカスタムリソースを取得。
	// 候補のノードの NodeAllocationState のカスタムリソースがない場合は、
	// そのノードで割り当て可能なリソースがないことを意味するので、
	// UnsuitableNodes に候補のノードを追加して返す
	if err := client.Get(); err != nil {
		logger.V(2).Info("Error retrieving NodeResourceAllocation of potential node, so return potential node as unsuitable node", "Error", err.Error())
		for _, ca := range allcas {
			ca.UnsuitableNodes = append(ca.UnsuitableNodes, potentialNode)
		}
		return nil
	}

	// 候補のノードのリソース割り当ての状態が Ready でない場合は、
	// そのノードで割り当て可能なリソースがないことを意味するので、
	// UnsuitableNodes に候補のノードを追加して返す
	if crd.Status != nascrd.NodeAllocationStateStatusReady {
		logger.V(2).Info("Potential node is not ready, so return potential node as unsuitable node")
		for _, ca := range allcas {
			ca.UnsuitableNodes = append(ca.UnsuitableNodes, potentialNode)
		}
		return nil
	}

	// 候補のノードの NodeAllocationState のカスタムリソースの割り当て済みの Fake リソースの情報を必要なら初期化
	if crd.Spec.AllocatedClaims == nil {
		crd.Spec.AllocatedClaims = make(map[string]nascrd.AllocatedDevices)
	}

	// リソースの種類 (e.g. Fake, GPU, ...) 毎に割り当て要求を整理する
	perKindCas := make(map[string][]*controller.ClaimAllocation)
	for _, ca := range allcas {
		var kind string
		switch ca.ClaimParameters.(type) {
		case *fakecrd.FakeClaimParametersSpec:
			kind = fakecrd.FakeClaimParametersKind
		}
		perKindCas[kind] = append(perKindCas[kind], ca)
	}
	// リソースの種類毎に割り当てられないノードを探すが、今回は Fake リソースのみ対象とする
	for _, kind := range []string{fakecrd.FakeClaimParametersKind} {
		var err error
		switch kind {
		case fakecrd.FakeClaimParametersKind:
			// Fake リソースの割り当て要求のみに限定して、対象のノードに割り当て可能か見ていく
			logger.V(4).Info("Check to see if the potential node is inappropriate for a Fake claim")
			err = d.fake.UnsuitableNodeForFakeClaim(ctx, crd, pod, perKindCas[kind], allcas, potentialNode)
		}
		if err != nil {
			return fmt.Errorf("error processing %q: %v", kind, err)
		}
	}

	return nil
}

func getSelectedNode(ctx context.Context, claim *resourcev1.ResourceClaim) string {
	logger := klog.FromContext(ctx)
	if claim.Status.Allocation == nil {
		logger.Info("Resource claim allocation status is nil. Cannot get selected node")
		return ""
	}
	if claim.Status.Allocation.AvailableOnNodes == nil {
		logger.Info("AvailaleOnNodes for resource claim allocation status is nil. Cannot get selected node.")
		return ""
	}
	if claim.Status.Allocation.AvailableOnNodes.NodeSelectorTerms == nil {
		logger.Info("Node selector terms for resource claim allocation status is nil. Cannot get selected node.")
		return ""
	}
	if claim.Status.Allocation.AvailableOnNodes.NodeSelectorTerms[0].MatchFields == nil {
		logger.Info("Node selector match fields for resource claim allocation status is nil. Cannot get selected node.")
		return ""
	}
	if claim.Status.Allocation.AvailableOnNodes.NodeSelectorTerms[0].MatchFields[0].Values == nil {
		logger.Info("Node selector match fields values for resource claim allocation status is nil. Cannot get selected node.")
		return ""
	}
	return claim.Status.Allocation.AvailableOnNodes.NodeSelectorTerms[0].MatchFields[0].Values[0]
}

func unique(s []string) []string {
	set := make(map[string]struct{})
	var news []string
	for _, str := range s {
		if _, ok := set[str]; !ok {
			set[str] = struct{}{}
			news = append(news, str)
		}
	}
	return news
}
