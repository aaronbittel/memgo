package main

import (
	"fmt"
	"sync"
	"time"
)

// TODO: use !now.Before(v.expiredAt) to expire an item at the expire time
// TODO: implement max cache size

const (
	maxValueSize = 1024 * 1024 // 1 MiB
)

var errValueTooLarge = fmt.Errorf("value exceeds maximum size: %d", maxValueSize)

type value struct {
	data      []byte
	flags     uint16
	expiredAt time.Time
}

func (v value) isExpired() bool {
	if v.expiredAt.IsZero() {
		return false
	}

	return v.expiredAt.Before(time.Now())
}

type store struct {
	mu    sync.Mutex
	store map[string]value
}

func newStore() *store {
	return &store{store: make(map[string]value)}
}

func (cm *store) get(key string) (value, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	val, ok := cm.store[key]

	if !ok {
		return value{}, false
	}

	if val.isExpired() {
		delete(cm.store, key)
		return value{}, false
	}
	return val, true
}

func (cm *store) set(key string, value value) error {
	if err := validateValueLen(len(value.data)); err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.store[key] = value
	return nil
}

func (cm *store) add(key string, value value) (added bool, err error) {
	if err := validateValueLen(len(value.data)); err != nil {
		return false, err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, exists := cm.store[key]
	if !exists {
		cm.store[key] = value
		return true, nil
	}

	if cur.isExpired() {
		cm.store[key] = value
		return true, nil
	}

	return false, nil
}

func (cm *store) replace(key string, value value) (replaced bool, err error) {
	if err := validateValueLen(len(value.data)); err != nil {
		return false, err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok || cur.isExpired() {
		return false, nil
	}

	cm.store[key] = value
	return true, nil
}

func (cm *store) append(key string, valueData []byte) (appended bool, err error) {
	if err := validateValueLen(len(valueData)); err != nil {
		return false, err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok || cur.isExpired() {
		return false, nil
	}

	if len(valueData) > maxValueSize-len(cur.data) {
		return false, errValueTooLarge
	}

	data := make([]byte, 0, len(cur.data)+len(valueData))
	data = append(data, cur.data...)
	data = append(data, valueData...)
	cur.data = data

	cm.store[key] = cur

	return true, nil
}

func (cm *store) prepend(key string, valueData []byte) (prepended bool, err error) {
	if err := validateValueLen(len(valueData)); err != nil {
		return false, err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cur, ok := cm.store[key]
	if !ok || cur.isExpired() {
		return false, nil
	}

	if len(valueData) > maxValueSize-len(cur.data) {
		return false, errValueTooLarge
	}

	data := make([]byte, 0, len(cur.data)+len(valueData))
	data = append(data, valueData...)
	data = append(data, cur.data...)
	cur.data = data

	cm.store[key] = cur

	return true, nil
}

func validateValueLen(n int) error {
	if n > maxValueSize {
		return errValueTooLarge
	}
	return nil
}
