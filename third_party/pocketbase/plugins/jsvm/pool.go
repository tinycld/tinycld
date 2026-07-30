package jsvm

import (
	"sync"
	"time"

	"github.com/grafana/sobek"
)

type poolItem struct {
	mux  sync.Mutex
	busy bool
	vm   *sobek.Runtime
}

type vmsPool struct {
	mux     sync.RWMutex
	factory func() *sobek.Runtime
	items   []*poolItem

	// budget bounds each run's execution (see Config.ExecTimeout); 0 = none.
	// Without it a single runaway handler occupies its executor forever, and
	// pool-size runaways wedge every hook in the app.
	budget time.Duration
}

// newPool creates a new pool with pre-warmed vms generated from the specified factory.
func newPool(size int, budget time.Duration, factory func() *sobek.Runtime) *vmsPool {
	pool := &vmsPool{
		factory: factory,
		items:   make([]*poolItem, size),
		budget:  budget,
	}

	for i := 0; i < size; i++ {
		vm := pool.factory()
		pool.items[i] = &poolItem{vm: vm}
	}

	return pool
}

// run executes "call" with a vm created from the pool
// (either from the buffer or a new one if all buffered vms are busy)
func (p *vmsPool) run(call func(vm *sobek.Runtime) error) error {
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
		vm := p.factory()
		return runBudgeted(vm, p.budget, func() error { return call(vm) })
	}

	execErr := runBudgeted(freeItem.vm, p.budget, func() error { return call(freeItem.vm) })

	// "free" the vm
	freeItem.mux.Lock()
	freeItem.busy = false
	freeItem.mux.Unlock()

	return execErr
}
