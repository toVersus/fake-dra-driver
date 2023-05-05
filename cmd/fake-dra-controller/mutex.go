package main

import "sync"

type PerNodeMutex struct {
	sync.Mutex
	submutex map[string]*sync.Mutex
}

func NewPerNodeMutex() *PerNodeMutex {
	return &PerNodeMutex{
		submutex: make(map[string]*sync.Mutex),
	}
}

func (m *PerNodeMutex) Get(node string) *sync.Mutex {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()
	if m.submutex[node] == nil {
		m.submutex[node] = &sync.Mutex{}
	}
	return m.submutex[node]
}
