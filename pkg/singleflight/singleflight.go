package singleflight

import "sync"

type call[V any] struct {
	value V
	err   error
	wg    sync.WaitGroup
}

type Group[K comparable, V any] struct {
	calls map[K]*call[V]
	mx    sync.Mutex
}

func (g *Group[K, V]) Do(key K, fn func() (V, error)) (V, error) {
	g.mx.Lock()
	if g.calls == nil {
		g.calls = make(map[K]*call[V])
	}
	if call, ok := g.calls[key]; ok {
		g.mx.Unlock()
		call.wg.Wait()
		return call.value, call.err
	}

	call := &call[V]{}
	call.wg.Add(1)
	g.calls[key] = call
	g.mx.Unlock()
	call.value, call.err = fn()

	g.mx.Lock()
	defer g.mx.Unlock()
	call.wg.Done()
	delete(g.calls, key)
	return call.value, call.err
}
