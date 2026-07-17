package main

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendConcurrent(t *testing.T) {
	ts := newTestServer(t)
	ts.store.set("result", value{data: []byte("")})
	ts.serve(t)

	const (
		clients    = 10
		iterations = 32
	)

	start := make(chan struct{})
	errs := make(chan error, clients)

	var wg sync.WaitGroup

	for c := range clients {
		wg.Go(func() {
			tce, err := newTestClientE(ts.addr())
			if err != nil {
				errs <- fmt.Errorf("creating client failed: client %d err: %v", c, err)
				return
			}

			<-start

			for i := range iterations {
				if err := tce.send("append result 0 0 1\r\na\r\n"); err != nil {
					errs <- fmt.Errorf("sending append failed: client %d (iteration: %d) err: %v", c, i, err)
					return
				}
				want := "STORED\r\n"
				got, err := tce.recv(want)
				if err != nil {
					errs <- fmt.Errorf("receiving response failed: client %d (iteration: %d) err: %v", c, i, err)
					return
				}
				if want != got {
					errs <- fmt.Errorf("expected %q, got %q: client %d (iteration: %d)", want, got, c, i)
					return
				}
			}
		})
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	wantCount := clients * iterations
	val, ok := ts.store.get("result")

	require.True(t, ok, "no value with key \"result\" in store")
	assert.Equal(t, wantCount, len(val.data), "expected same length for value")

	gotCount := bytes.Count(val.data, []byte{'a'})
	assert.Equalf(t, wantCount, gotCount, "expected count of 'a': %d, got %d", wantCount, gotCount)
}

func TestConcurrent(t *testing.T) {
	ts := newTestServer(t)
	ts.store.set("counter", value{data: []byte("0")})
	ts.serve(t)

	const (
		clients    = 5
		iterations = 30
	)

	start := make(chan struct{})
	errs := make(chan error, clients)

	var wg sync.WaitGroup

	for i := range clients {
		wg.Go(func() {
			tce, err := newTestClientE(ts.addr())
			if err != nil {
				errs <- fmt.Errorf("client %d: connect: %w", i, err)
				return
			}
			defer tce.conn.Close()

			<-start

			for it := range iterations {
				if err := tce.send("get counter\r\n"); err != nil {
					errs <- fmt.Errorf("client %d iteration %d: get: %w", i, it, err)
					return
				}

				msg := fmt.Sprintf("set counter 0 0 1\r\n%s\r\n", strconv.Itoa(i))
				if err := tce.send(msg); err != nil {
					errs <- fmt.Errorf("client %d iteration %d: get: %w", i, it, err)
					return
				}
			}
		})
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
