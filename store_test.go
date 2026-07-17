package main

import (
	"bytes"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreExpiry(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newStore()

			ttl := 10 * time.Second
			s.set("test", value{expiredAt: time.Now().Add(ttl)})

			_, exists := s.get("test")
			require.True(t, exists, "value missing immediately after insertion")

			time.Sleep(ttl)

			_, exists = s.get("test")
			require.True(t, exists, "value expired exactly at its deadline")

			time.Sleep(time.Nanosecond)

			_, exists = s.get("test")
			require.False(t, exists, "value remained after its deadline")
		})
	})

	t.Run("negative", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newStore()

			ttl := -time.Second
			s.set("test", value{expiredAt: time.Now().Add(ttl)})

			_, exists := s.get("test")
			require.False(t, exists, "value should be instantly expired")
		})
	})

	t.Run("zero", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newStore()

			s.set("test", value{expiredAt: time.Time{}})

			_, exists := s.get("test")
			require.True(t, exists, "value missing immediately after insertion")

			time.Sleep(24 * 365 * 10 * time.Hour)

			_, exists = s.get("test")
			require.True(t, exists, "value should still be in store")
		})
	})

	t.Run("lazy removal", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newStore()

			ttl := 5 * time.Minute

			s.set("test", value{expiredAt: time.Now().Add(ttl)})

			time.Sleep(ttl + time.Nanosecond)

			_, exists := rawStoreValue(s, "test")
			require.True(t, exists, "expired value was removed before get")

			_, exists = s.get("test")
			require.False(t, exists, "get returned an expired value")

			_, exists = rawStoreValue(s, "test")
			require.False(t, exists, "expired value remains after get")
		})
	})
}

func TestStoreSetAndGet(t *testing.T) {
	s := newStore()

	s.set("test", value{data: []byte("first"), flags: 10})
	s.set("test", value{data: []byte("second"), flags: 20})

	got, ok := s.get("test")
	require.True(t, ok, "get() returned ok=false; want true")

	want := value{data: []byte("second"), flags: 20}
	requireEqualValue(t, want, got)
}

func TestStoreGetMissingValue(t *testing.T) {
	s := newStore()

	got, ok := s.get("missing")
	require.Falsef(t, ok, "get() returned (%#v, true); want (zero value, false)", got)

	requireEqualValue(t, got, value{})
}

func TestStoreAdd(t *testing.T) {
	t.Run("adds missing value", func(t *testing.T) {
		s := newStore()

		want := value{data: []byte("new value"), flags: 10}

		require.True(t, s.add("test", want), "add() returned false; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, want, got)
	})

	t.Run("does not replace live value", func(t *testing.T) {
		s := newStore()

		original := value{data: []byte("original"), flags: 10}

		s.set("test", original)

		added := s.add("test", value{data: []byte("replacement"), flags: 20})
		require.False(t, added, "add() returned true for an existing live value; want false")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, got, original)
	})

	t.Run("replaces expired value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newStore()

			ttl := time.Minute

			s.set("test", value{
				data:      []byte("expired"),
				flags:     10,
				expiredAt: time.Now().Add(ttl),
			})

			time.Sleep(ttl + time.Nanosecond)

			replacement := value{
				data:      []byte("replacement"),
				flags:     20,
				expiredAt: time.Now().Add(ttl),
			}

			require.True(t, s.add("test", replacement), "add() returned false for an expired value; want true")

			got, ok := s.get("test")
			require.True(t, ok, "get() returned ok=false; want true")

			requireEqualValue(t, got, replacement)
		})
	})
}

func TestStoreReplace(t *testing.T) {
	t.Run("rejects missing value", func(t *testing.T) {
		s := newStore()

		replaced := s.replace("missing", value{data: []byte("replacement")})
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
			expiredAt: time.Now().Add(time.Minute),
		}

		require.True(t, s.replace("test", replacement), "replace() returned false for a live value; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, got, replacement)
	})

	t.Run("rejects expired value without replacing it", func(t *testing.T) {
		s := newStore()

		expired := value{
			data:      []byte("expired"),
			flags:     10,
			expiredAt: time.Now().Add(-time.Second),
		}

		s.set("test", expired)

		replaced := s.replace("test", value{
			data:  []byte("replacement"),
			flags: 20,
		})

		require.False(t, replaced, "replace() returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.True(t, exists, "replace() removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
}

func TestStoreAppend(t *testing.T) {
	t.Run("value exists", func(t *testing.T) {
		s := newStore()

		expiredAt := time.Now().Add(time.Minute)

		s.set("test", value{
			data:      []byte("hello"),
			flags:     42,
			expiredAt: expiredAt,
		})

		require.Truef(t, s.append("test", []byte(" world")), "returned false; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		want := value{data: []byte("hello world"), flags: 42, expiredAt: expiredAt}
		requireEqualValue(t, want, got)
	})

	t.Run("missing value", func(t *testing.T) {
		s := newStore()

		stored := s.append("missing", []byte("value"))

		require.Falsef(t, stored, "returned true for a missing value; want false")

		_, exists := rawStoreValue(s, "missing")
		require.Falsef(t, exists, "created a missing value")
	})

	t.Run("expired value", func(t *testing.T) {
		s := newStore()

		expired := value{
			data:      []byte("original"),
			flags:     42,
			expiredAt: time.Now().Add(-time.Second),
		}

		s.set("test", expired)

		stored := s.append("test", []byte("new data"))

		require.Falsef(t, stored, "returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.Truef(t, exists, "removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
}

func TestStorePrepend(t *testing.T) {
	t.Run("value exists", func(t *testing.T) {
		s := newStore()

		expiredAt := time.Now().Add(time.Minute)

		s.set("test", value{
			data:      []byte("world"),
			flags:     42,
			expiredAt: expiredAt,
		})

		require.Truef(t, s.prepend("test", []byte("hello ")), "returned false; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		want := value{data: []byte("hello world"), flags: 42, expiredAt: expiredAt}
		requireEqualValue(t, want, got)
	})

	t.Run("missing value", func(t *testing.T) {
		s := newStore()

		stored := s.prepend("missing", []byte("value"))

		require.Falsef(t, stored, "returned true for a missing value; want false")

		_, exists := rawStoreValue(s, "missing")
		require.Falsef(t, exists, "created a missing value")
	})

	t.Run("expired value", func(t *testing.T) {
		s := newStore()

		expired := value{
			data:      []byte("original"),
			flags:     42,
			expiredAt: time.Now().Add(-time.Second),
		}

		s.set("test", expired)

		stored := s.prepend("test", []byte("new data"))

		require.Falsef(t, stored, "returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.Truef(t, exists, "removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
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
			results <- s.append("test", []byte{'x'})
		})
	}

	close(start)
	wg.Wait()
	close(results)

	for stored := range results {
		require.True(t, stored, "append() returned false during concurrent access")
	}

	got, ok := s.get("test")
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
