package nodenet

import "sync"

type lockEntry struct {
	mu      sync.Mutex
	waiters int
}

// keyedMutex provides per-key mutual exclusion.
// The zero value is ready to use.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
}

func (km *keyedMutex) Lock(key string) {
	km.mu.Lock()
	if km.locks == nil {
		km.locks = make(map[string]*lockEntry)
	}
	e, ok := km.locks[key]
	if !ok {
		e = &lockEntry{}
		km.locks[key] = e
	}
	e.waiters++
	km.mu.Unlock()
	e.mu.Lock()
}

func (km *keyedMutex) Unlock(key string) {
	km.mu.Lock()
	e := km.locks[key]
	e.waiters--
	if e.waiters == 0 {
		delete(km.locks, key)
	}
	km.mu.Unlock()
	e.mu.Unlock()
}
