package main

import (
	"sync"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
)

// 遅延割り当てでリソースを割り当てる際に、割り当てを待っているリソースの情報を保持する。
// UnsuitableNodes API でリ割り当てできないノードを探す際に、この情報を参照して
// 割り当て済みのリソースを再度割り当てないようにする。
type PerNodeAllocatedClaim struct {
	sync.RWMutex
	// ResourceClaim の UID, ノード単位で割り当て済みのデバイスの情報を保持している。
	// リソースが割り当てられたら、対応する ResourceClaim の UID とノードの組み合わせの情報を削除する。
	allocations map[string]map[string]nascrd.AllocatedDevices
}

func NewPerNodeAllocatedClaims() *PerNodeAllocatedClaim {
	return &PerNodeAllocatedClaim{
		allocations: make(map[string]map[string]nascrd.AllocatedDevices),
	}
}

func (p *PerNodeAllocatedClaim) Exists(claimUID, node string) bool {
	p.RLock()
	defer p.RUnlock()

	if _, ok := p.allocations[claimUID]; !ok {
		return false
	}

	_, ok := p.allocations[claimUID][node]
	return ok
}

func (p *PerNodeAllocatedClaim) Get(claimUID, node string) nascrd.AllocatedDevices {
	p.RLock()
	defer p.RUnlock()

	if !p.Exists(claimUID, node) {
		return nascrd.AllocatedDevices{}
	}

	return p.allocations[claimUID][node]
}

func (p *PerNodeAllocatedClaim) VisitNode(node string, visitor func(claimUID string, allocation nascrd.AllocatedDevices)) {
	p.RLock()

	for claimUID := range p.allocations {
		if allocation, ok := p.allocations[claimUID][node]; ok {
			p.RUnlock()
			visitor(claimUID, allocation)
			p.RLock()
		}
	}
	p.RUnlock()
}

func (p *PerNodeAllocatedClaim) Visit(visitor func(claimUID, node string, allocation nascrd.AllocatedDevices)) {
	p.RLock()

	for claimUID := range p.allocations {
		for node, allocation := range p.allocations[claimUID] {
			p.RUnlock()
			visitor(claimUID, node, allocation)
			p.RLock()
		}
	}
	p.RUnlock()
}

func (p *PerNodeAllocatedClaim) Set(claimUID, node string, devices nascrd.AllocatedDevices) {
	p.Lock()
	defer p.Unlock()

	_, ok := p.allocations[claimUID]
	if !ok {
		p.allocations[claimUID] = make(map[string]nascrd.AllocatedDevices)
	}
	p.allocations[claimUID][node] = devices
}

func (p *PerNodeAllocatedClaim) RemoveNode(claimUID, node string) {
	p.Lock()
	defer p.Unlock()

	if _, ok := p.allocations[claimUID]; !ok {
		return
	}

	delete(p.allocations[claimUID], node)
}

func (p *PerNodeAllocatedClaim) Remove(claimUID string) {
	p.Lock()
	defer p.Unlock()

	delete(p.allocations, claimUID)
}
