package main

import (
	"bytes"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreSetValueLength(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		s := newStore()

		data := []byte{}
		require.NoError(t, s.set("test", value{data: data}))
	})

	t.Run("maxValueSize", func(t *testing.T) {
		s := newStore()

		data := bytes.Repeat([]byte{'a'}, maxValueSize)
		require.NoError(t, s.set("test", value{data: data}))
	})

	t.Run("value length too large", func(t *testing.T) {
		s := newStore()

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)
		require.Error(t, s.set("test", value{data: data}), errValueTooLarge)
	})
}

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

		added, err := s.add("test", want)
		require.NoError(t, err)
		require.True(t, added, "add() returned false; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		requireEqualValue(t, want, got)
	})

	t.Run("does not replace live value", func(t *testing.T) {
		s := newStore()

		original := value{data: []byte("original"), flags: 10}

		s.set("test", original)

		added, err := s.add("test", value{data: []byte("replacement"), flags: 20})
		require.NoError(t, err)
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

			added, err := s.add("test", replacement)
			require.NoError(t, err)
			require.True(t, added, "add() returned false for an expired value; want true")

			got, ok := s.get("test")
			require.True(t, ok, "get() returned ok=false; want true")

			requireEqualValue(t, got, replacement)
		})
	})
}

func TestStoreAddValueLength(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		s := newStore()

		data := []byte{}
		added, err := s.add("test", value{data: data})
		require.NoError(t, err)
		require.True(t, added)
	})

	t.Run("maxValueSize", func(t *testing.T) {
		s := newStore()

		data := bytes.Repeat([]byte{'a'}, maxValueSize)

		added, err := s.add("test", value{data: data})
		require.NoError(t, err)
		require.True(t, added)
	})

	t.Run("value length too large", func(t *testing.T) {
		s := newStore()

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)

		_, err := s.add("test", value{data: data})
		require.ErrorIs(t, err, errValueTooLarge)
	})
}

func TestStoreReplace(t *testing.T) {
	t.Run("rejects missing value", func(t *testing.T) {
		s := newStore()

		replaced, err := s.replace("missing", value{data: []byte("replacement")})
		require.NoError(t, err)
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

		replaced, err := s.replace("test", replacement)
		require.NoError(t, err)
		require.True(t, replaced, "replace() returned false for a live value; want true")

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

		replaced, err := s.replace("test", value{
			data:  []byte("replacement"),
			flags: 20,
		})

		require.NoError(t, err)
		require.False(t, replaced, "replace() returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.True(t, exists, "replace() removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
}

func TestStoreReplaceValueLength(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := []byte{}
		replaced, err := s.replace("test", value{data: data})
		require.NoError(t, err)
		require.True(t, replaced)
	})

	t.Run("maxValueSize", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := bytes.Repeat([]byte{'a'}, maxValueSize)

		replaced, err := s.replace("test", value{data: data})
		require.NoError(t, err)
		require.True(t, replaced)
	})

	t.Run("value length too large", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)

		_, err := s.replace("test", value{data: data})
		require.ErrorIs(t, err, errValueTooLarge)
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

		appended, err := s.append("test", []byte(" world"))
		require.NoError(t, err)
		require.Truef(t, appended, "returned false; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		want := value{data: []byte("hello world"), flags: 42, expiredAt: expiredAt}
		requireEqualValue(t, want, got)
	})

	t.Run("missing value", func(t *testing.T) {
		s := newStore()

		appended, err := s.append("missing", []byte("value"))
		require.NoError(t, err)

		require.Falsef(t, appended, "returned true for a missing value; want false")

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

		appended, err := s.append("test", []byte("new data"))
		require.NoError(t, err)

		require.Falsef(t, appended, "returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.Truef(t, exists, "removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
}

func TestStoreAppendValueLength(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := []byte{}
		appended, err := s.append("test", data)
		require.NoError(t, err)
		require.True(t, appended)
	})

	t.Run("maxValueSize", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := bytes.Repeat([]byte{'a'}, maxValueSize)

		appended, err := s.append("test", data)
		require.NoError(t, err)
		require.True(t, appended)
	})

	t.Run("value length too large", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)

		_, err := s.append("test", data)
		require.ErrorIs(t, err, errValueTooLarge)
	})

	t.Run("result exactly maxValueSize", func(t *testing.T) {
		s := newStore()

		require.NoError(t, s.set("test", value{
			data: bytes.Repeat([]byte{'a'}, maxValueSize-1),
		}))

		appended, err := s.append("test", []byte{'b'})

		require.NoError(t, err)
		require.True(t, appended)

		got, ok := s.get("test")
		require.True(t, ok)
		require.Len(t, got.data, maxValueSize)
	})

	t.Run("combined value too large", func(t *testing.T) {
		s := newStore()

		original := bytes.Repeat([]byte{'a'}, maxValueSize)

		require.NoError(t, s.set("test", value{
			data: original,
		}))

		appended, err := s.append("test", []byte{'b'})

		require.ErrorIs(t, err, errValueTooLarge)
		require.False(t, appended)

		// The failed operation must not modify the stored value.
		got, ok := s.get("test")
		require.True(t, ok)
		require.Equal(t, original, got.data)
	})

	t.Run("incoming value alone too large", func(t *testing.T) {
		s := newStore()
		require.NoError(t, s.set("test", value{}))

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)

		appended, err := s.append("test", data)

		require.ErrorIs(t, err, errValueTooLarge)
		require.False(t, appended)
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

		prepended, err := s.prepend("test", []byte("hello "))

		require.NoError(t, err)
		require.Truef(t, prepended, "returned false; want true")

		got, ok := s.get("test")
		require.True(t, ok, "get() returned ok=false; want true")

		want := value{data: []byte("hello world"), flags: 42, expiredAt: expiredAt}
		requireEqualValue(t, want, got)
	})

	t.Run("missing value", func(t *testing.T) {
		s := newStore()

		prepended, err := s.prepend("missing", []byte("value"))
		require.NoError(t, err)

		require.Falsef(t, prepended, "returned true for a missing value; want false")

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

		prepended, err := s.prepend("test", []byte("new data"))
		require.NoError(t, err)

		require.Falsef(t, prepended, "returned true for an expired value; want false")

		got, exists := rawStoreValue(s, "test")
		require.Truef(t, exists, "removed the expired value unexpectedly")

		requireEqualValue(t, got, expired)
	})
}

func TestStorePrependValueLength(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := []byte{}
		prepended, err := s.prepend("test", data)
		require.NoError(t, err)
		require.True(t, prepended)
	})

	t.Run("maxValueSize", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := bytes.Repeat([]byte{'a'}, maxValueSize)

		prepended, err := s.prepend("test", data)
		require.NoError(t, err)
		require.True(t, prepended)
	})

	t.Run("value length too large", func(t *testing.T) {
		s := newStore()
		s.set("test", value{})

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)

		_, err := s.prepend("test", data)
		require.ErrorIs(t, err, errValueTooLarge)
	})

	t.Run("result exactly maxValueSize", func(t *testing.T) {
		s := newStore()

		require.NoError(t, s.set("test", value{
			data: bytes.Repeat([]byte{'a'}, maxValueSize-1),
		}))

		prepended, err := s.prepend("test", []byte{'b'})

		require.NoError(t, err)
		require.True(t, prepended)

		got, ok := s.get("test")
		require.True(t, ok)
		require.Len(t, got.data, maxValueSize)
	})

	t.Run("combined value too large", func(t *testing.T) {
		s := newStore()

		original := bytes.Repeat([]byte{'a'}, maxValueSize)

		require.NoError(t, s.set("test", value{
			data: original,
		}))

		prepended, err := s.prepend("test", []byte{'b'})

		require.ErrorIs(t, err, errValueTooLarge)
		require.False(t, prepended)

		// The failed operation must not modify the stored value.
		got, ok := s.get("test")
		require.True(t, ok)
		require.Equal(t, original, got.data)
	})

	t.Run("incoming value alone too large", func(t *testing.T) {
		s := newStore()
		require.NoError(t, s.set("test", value{}))

		data := bytes.Repeat([]byte{'a'}, maxValueSize+1)

		prepended, err := s.prepend("test", data)

		require.ErrorIs(t, err, errValueTooLarge)
		require.False(t, prepended)
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
			appended, err := s.append("test", []byte{'x'})
			require.NoError(t, err)
			results <- appended
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
