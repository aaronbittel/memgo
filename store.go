package main

import (
	"sync"
	"time"
)

// TODO: use !now.Before(v.expiredAt) to expire an item at the expire time

type value struct {
	data      []byte
	flags     uint16
	expiredAt time.Time
}

func (v value) isExpired(t time.Time) bool {
	if v.expiredAt.IsZero() {
		return false
	}

	return v.expiredAt.Before(t)
}

type store struct {
	mu    sync.Mutex
	store map[string]value
}

func newStore() *store {
	return &store{store: make(map[string]value)}
}

func (cm *store) get(key string) (value, bool) {
	now := time.Now()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	val, ok := cm.store[key]

	if !ok {
		return value{}, false
	}

	if val.isExpired(now) {
		delete(cm.store, key)
		return value{}, false
	}
	return val, true
}

func (cm *store) set(key string, value value) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.store[key] = value
}

func (cm *store) add(key string, value value) bool {
	now := time.Now()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok {
		cm.store[key] = value
		return true
	}

	if cur.isExpired(now) {
		cm.store[key] = value
		return true
	}

	return false
}

func (cm *store) replace(key string, value value) bool {
	now := time.Now()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok || cur.isExpired(now) {
		return false
	}

	cm.store[key] = value
	return true
}

func (cm *store) append(key string, valueData []byte) bool {
	now := time.Now()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok || cur.isExpired(now) {
		return false
	}

	data := make([]byte, 0, len(cur.data)+len(valueData))
	data = append(data, cur.data...)
	data = append(data, valueData...)
	cur.data = data

	cm.store[key] = cur

	return true
}

func (cm *store) prepend(key string, valueData []byte) bool {
	now := time.Now()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok || cur.isExpired(now) {
		return false
	}

	data := make([]byte, 0, len(cur.data)+len(valueData))
	data = append(data, valueData...)
	data = append(data, cur.data...)
	cur.data = data

	cm.store[key] = cur

	return true
}
