package main

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicUpdates(t *testing.T) {
	const (
		clients    = 20
		iterations = 100
	)

	store := newConcurrentMap[string, int]()
	store.set("counter", 0)

	start := make(chan struct{})

	var wg sync.WaitGroup

	for range clients {
		wg.Go(func() {
			<-start

			for range iterations {
				store.update("counter", func(current int, exists bool) int {
					if exists {
						return current + 1
					}
					return 1
				})
			}
		})
	}

	close(start)
	wg.Wait()

	want := clients * iterations
	got, ok := store.get("counter")
	if !ok {
		t.Fatal("key 'counter' not found")
	}

	assert.Equal(t, want, got)
}
