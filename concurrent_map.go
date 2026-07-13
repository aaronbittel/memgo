package main

import "sync"

type concurrentMap[K comparable, V any] struct {
	mu    sync.RWMutex
	store map[K]V
}

func newConcurrentMap[K comparable, V any]() *concurrentMap[K, V] {
	return &concurrentMap[K, V]{store: make(map[K]V)}
}

func (cm *concurrentMap[K, V]) get(key K) (V, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	v, ok := cm.store[key]
	return v, ok
}

func (cm *concurrentMap[K, V]) set(key K, value V) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.store[key] = value
}

func (cm *concurrentMap[K, V]) delete(key K) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.store, key)
}

// update atomically replaces the value associated with key.
//
// fn executes while cm's write lock is held. It must not call get, set,
// or update on cm, and it should complete quickly.
func (cm *concurrentMap[K, V]) update(key K, fn func(current V, exists bool) V) V {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	current, exists := cm.store[key]
	next := fn(current, exists)
	cm.store[key] = next

	return next
}
