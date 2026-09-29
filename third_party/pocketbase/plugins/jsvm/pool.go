package jsvm

import (
	"sync"

	goja "github.com/grafana/sobek"
)

type poolItem struct {
	mux  sync.Mutex
	busy bool
	vm   *goja.Runtime
}

type vmsPool struct {
	mux     sync.RWMutex
	factory func() *goja.Runtime
	items   []*poolItem

	tinycld poolOptions // fork: see jsvm_tinycld.go
}

// newPool creates a new pool with pre-warmed vms generated from the specified factory.
func newPool(size int, factory func() *goja.Runtime) *vmsPool {
	pool := &vmsPool{
		factory: factory,
		items:   make([]*poolItem, size),
	}

	for i := 0; i < size; i++ {
		vm := pool.factory()
		pool.items[i] = &poolItem{vm: vm}
	}

	return pool
}

// run executes "call" with a vm created from the pool
// (either from the buffer or a new one if all buffered vms are busy)
func (p *vmsPool) run(call func(vm *goja.Runtime) error) error {
	p.mux.RLock()

	// try to find a free item
	var freeItem *poolItem
	for _, item := range p.items {
		item.mux.Lock()
		if item.busy {
			item.mux.Unlock()
			continue
		}
		item.busy = true
		item.mux.Unlock()
		freeItem = item
		break
	}

	p.mux.RUnlock()

	// create a new one-off item if of all of the pool items are currently busy
	//
	// note: if turned out not efficient we may change this in the future
	// by adding the created item in the pool with some timer for removal
	if freeItem == nil {
		return p.tinycld.run(p.factory(), call)
	}

	execErr := p.tinycld.run(freeItem.vm, call)

	// "free" the vm
	freeItem.mux.Lock()
	freeItem.busy = false
	freeItem.mux.Unlock()

	return execErr
}
