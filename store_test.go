package main

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var storeTestNow = time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)

func TestStoreSetAndGet(t *testing.T) {
	s := newStore()

	s.set("test", value{data: []byte("first"), flags: 10})
	s.set("test", value{data: []byte("second"), flags: 20})

	got, ok := s.get("test", storeTestNow)
	require.True(t, ok, "get() returned ok=false; want true")

	want := value{data: []byte("second"), flags: 20}
	requireEqualValue(t, want, got)
}

func TestStoreGetMissingValue(t *testing.T) {
	s := newStore()

	got, ok := s.get("missing", storeTestNow)
	require.Falsef(t, ok, "get() returned (%#v, true); want (zero value, false)", got)

	requireEqualValue(t, got, value{})
}

func TestStoreGetRemovesExpiredValue(t *testing.T) {
	s := newStore()

	s.set("test", value{
		data:      []byte("expired"),
		flags:     10,
		expiredAt: storeTestNow.Add(-time.Second),
	})

	got, ok := s.get("test", storeTestNow)
	require.False(t, ok, "get() returned (%#v, true); want (zero value, false)", got)

	requireEqualValue(t, got, value{})

	if _, exists := rawStoreValue(s, "test"); exists {
		t.Fatal("expired value remains in the underlying map; want it removed")
	}
}

func TestStoreAdd(t *testing.T) {
	t.Run("adds missing value", func(t *testing.T) {
		s := newStore()

		want := value{data: []byte("new value"), flags: 10}

		require.True(t, s.add("test", want, storeTestNow), "add() returned false; want true")

		got, ok := s.get("test", storeTestNow)
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, want, got)
	})

	t.Run("does not replace live value", func(t *testing.T) {
		s := newStore()

		original := value{
			data:      []byte("original"),
			flags:     10,
			expiredAt: storeTestNow.Add(time.Minute),
		}

		s.set("test", original)

		added := s.add("test", value{data: []byte("replacement"), flags: 20}, storeTestNow)
		require.False(t, added, "add() returned true for an existing live value; want false")

		got, ok := s.get("test", storeTestNow)
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, got, original)
	})

	t.Run("replaces expired value", func(t *testing.T) {
		s := newStore()

		s.set("test", value{
			data:      []byte("expired"),
			flags:     10,
			expiredAt: storeTestNow.Add(-time.Second),
		})

		replacement := value{
			data:      []byte("replacement"),
			flags:     20,
			expiredAt: storeTestNow.Add(time.Minute),
		}

		require.True(t, s.add("test", replacement, storeTestNow), "add() returned false for an expired value; want true")

		got, ok := s.get("test", storeTestNow)
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, got, replacement)
	})
}

func TestStoreReplace(t *testing.T) {
	t.Run("rejects missing value", func(t *testing.T) {
		s := newStore()

		replaced := s.replace("missing", value{data: []byte("replacement")}, storeTestNow)
		require.False(t, replaced, "replace() returned true for a missing value; want false")

		_, exists := rawStoreValue(s, "missing")
		require.False(t, exists, "replace() created a missing value")
	})

	t.Run("replaces live value", func(t *testing.T) {
		s := newStore()

		s.set("test", value{data: []byte("original"), flags: 10})

		replacement := value{
			data:      []byte("replacement"),
			flags:     20,
			expiredAt: storeTestNow.Add(time.Minute),
		}

		require.True(t, s.replace("test", replacement, storeTestNow), "replace() returned false for a live value; want true")

		got, ok := s.get("test", storeTestNow)
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, got, replacement)
	})

	t.Run("rejects expired value without replacing it", func(t *testing.T) {
		s := newStore()

		expired := value{
			data:      []byte("expired"),
			flags:     10,
			expiredAt: storeTestNow.Add(-time.Second),
		}

		s.set("test", expired)

		replaced := s.replace("test", value{
			data:  []byte("replacement"),
			flags: 20,
		}, storeTestNow)

		require.False(t, replaced, "replace() returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.True(t, exists, "replace() removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
}

func TestStoreAppendAndPrepend(t *testing.T) {
	tests := []struct {
		name      string
		initial   []byte
		addition  []byte
		operation func(*store, string, []byte, time.Time) bool
		wantData  []byte
	}{
		{
			name:      "append",
			initial:   []byte("hello"),
			addition:  []byte(" world"),
			operation: (*store).append,
			wantData:  []byte("hello world"),
		},
		{
			name:      "prepend",
			initial:   []byte("world"),
			addition:  []byte("hello "),
			operation: (*store).prepend,
			wantData:  []byte("hello world"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()

			expiredAt := storeTestNow.Add(time.Minute)

			s.set("test", value{
				data:      tt.initial,
				flags:     42,
				expiredAt: expiredAt,
			})

			require.Truef(t, tt.operation(s, "test", tt.addition, storeTestNow), "%s() returned false; want true", tt.name)

			got, ok := s.get("test", storeTestNow)
			require.True(t, ok, "get() returned ok=false; want true")

			want := value{data: tt.wantData, flags: 42, expiredAt: expiredAt}
			requireEqualValue(t, want, got)
		})
	}
}

func TestStoreAppendAndPrependRejectMissingValue(t *testing.T) {
	operations := []struct {
		name      string
		operation func(*store, string, []byte, time.Time) bool
	}{
		{
			name:      "append",
			operation: (*store).append,
		},
		{
			name:      "prepend",
			operation: (*store).prepend,
		},
	}

	for _, tt := range operations {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()

			stored := tt.operation(
				s,
				"missing",
				[]byte("value"),
				storeTestNow,
			)

			require.Falsef(t, stored, "%s() returned true for a missing value; want false", tt.name)

			_, exists := rawStoreValue(s, "missing")
			require.Falsef(t, exists, "%s() created a missing value", tt.name)
		})
	}
}

func TestStoreAppendAndPrependRejectExpiredValue(t *testing.T) {
	operations := []struct {
		name      string
		operation func(*store, string, []byte, time.Time) bool
	}{
		{
			name:      "append",
			operation: (*store).append,
		},
		{
			name:      "prepend",
			operation: (*store).prepend,
		},
	}

	for _, tt := range operations {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()

			expired := value{
				data:      []byte("original"),
				flags:     42,
				expiredAt: storeTestNow.Add(-time.Second),
			}

			s.set("test", expired)

			stored := tt.operation(
				s,
				"test",
				[]byte("new data"),
				storeTestNow,
			)

			require.Falsef(t, stored, "%s() returned true for an expired value; want false", tt.name)

			got, exists := rawStoreValue(s, "test")
			require.Truef(t, exists, "%s() removed the expired value unexpectedly", tt.name)

			requireEqualValue(t, got, expired)
		})
	}
}

func TestStoreConcurrentAppend(t *testing.T) {
	const goroutines = 100

	s := newStore()
	s.set("test", value{data: []byte{}})

	start := make(chan struct{})
	results := make(chan bool, goroutines)

	var wg sync.WaitGroup

	for range goroutines {
		wg.Go(func() {
			<-start
			results <- s.append("test", []byte{'x'}, storeTestNow)
		})
	}

	close(start)
	wg.Wait()
	close(results)

	for stored := range results {
		require.True(t, stored, "append() returned false during concurrent access")
	}

	got, ok := s.get("test", storeTestNow)
	require.True(t, ok, "get() returned ok=false; want true")

	if gotLength := len(got.data); gotLength != goroutines {
		t.Fatalf("stored data length = %d; want %d", gotLength, goroutines)
	}
}

func requireEqualValue(t *testing.T, want, got value) {
	t.Helper()

	if !bytes.Equal(got.data, want.data) {
		t.Errorf("data = %q; want %q", got.data, want.data)
	}

	if got.flags != want.flags {
		t.Errorf("flags = %d; want %d", got.flags, want.flags)
	}

	if !got.expiredAt.Equal(want.expiredAt) {
		t.Errorf(
			"expiredAt = %v; want %v",
			got.expiredAt,
			want.expiredAt,
		)
	}
}

func rawStoreValue(s *store, key string) (value, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	got, exists := s.store[key]
	return got, exists
}
