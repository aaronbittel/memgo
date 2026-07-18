package main

import (
	"bufio"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Run("value", func(t *testing.T) {
		ts := newTestServer(t)

		err := ts.store.set("test", value{data: []byte("hello, world!")})
		require.NoError(t, err)

		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 13\r\nhello, world!\r\nEND\r\n")
	})

	t.Run("missing value", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "get missing\r\n")
		tc.requireEnd(t)
	})

	t.Run("crlf in value", func(t *testing.T) {
		ts := newTestServer(t)

		err := ts.store.set("test", value{data: []byte("hello\r\nworld")})
		require.NoError(t, err)

		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 12\r\nhello\r\nworld\r\nEND\r\n")
	})
}

func TestSet(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		wantResponse string
		want         value
	}{
		{
			name:    "stores value",
			command: "set test 0 0 4\r\n1234\r\n",
			want:    value{data: []byte("1234")},
		},
		{
			name:    "stores flags",
			command: "set test 10 0 4\r\n1234\r\n",
			want: value{
				data:  []byte("1234"),
				flags: 10,
			},
		},
		{
			name:    "crlf in value",
			command: "set test 0 0 6\r\n12\r\n34\r\n",
			want:    value{data: []byte("12\r\n34")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			ts.serve(t)

			tc := newTestClient(t, ts.addr())

			tc.send(t, tt.command)
			tc.requireStored(t)

			ts.requireStoredValue(t, "test", tt.want)
		})
	}

	t.Run("no reply", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5 noreply\r\nhello\r\n")
		tc.requireNoResponse(t)
		ts.requireStoredValue(t, "test", value{data: []byte("hello")})
	})
}

func TestSetAndGet(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "stores value",
			command: "set test 0 0 5\r\nhello\r\n",
			want:    "VALUE test 0 5\r\nhello\r\nEND\r\n",
		},
		{
			name:    "flags",
			command: "set test 10 0 5\r\nhello\r\n",
			want:    "VALUE test 10 5\r\nhello\r\nEND\r\n",
		},
		{
			name:    "expiry time",
			command: "set test 0 20 5\r\nhello\r\n",
			want:    "VALUE test 0 5\r\nhello\r\nEND\r\n",
		},
		{
			name:    "value with crlf",
			command: "set test 0 20 12\r\nhello\r\nworld\r\n",
			want:    "VALUE test 0 12\r\nhello\r\nworld\r\nEND\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			ts.serve(t)

			tc := newTestClient(t, ts.addr())

			tc.send(t, tt.command)
			tc.requireStored(t)

			tc.send(t, "get test\r\n")
			tc.requireResponse(t, tt.want)
		})
	}

	t.Run("noreply", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5 noreply\r\nhello\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
	})
}

func TestSetExpiry(t *testing.T) {
	t.Run("negative", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newServer(testLogger(t))

			var sb strings.Builder
			err := s.handleCommand(&sb, bufio.NewReader(strings.NewReader("set test 0 -1 5\r\nhello\r\n")))
			require.NoError(t, err)
			want := "STORED\r\n"
			got := sb.String()
			require.Equal(t, want, got)

			sb.Reset()
			err = s.handleCommand(&sb, bufio.NewReader(strings.NewReader("get test\r\n")))
			require.NoError(t, err)
			want = "END\r\n"
			got = sb.String()
			require.Equal(t, want, got)
		})
	})

	t.Run("zero", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newServer(testLogger(t))

			var sb strings.Builder
			err := s.handleCommand(&sb, bufio.NewReader(strings.NewReader("set test 0 0 5\r\nhello\r\n")))
			require.NoError(t, err)
			want := "STORED\r\n"
			got := sb.String()
			require.Equal(t, want, got)

			sb.Reset()
			err = s.handleCommand(&sb, bufio.NewReader(strings.NewReader("get test\r\n")))
			require.NoError(t, err)
			want = "VALUE test 0 5\r\nhello\r\nEND\r\n"
			got = sb.String()
			require.Equal(t, want, got)

			time.Sleep(24 * 365 * 10 * time.Hour)

			sb.Reset()
			err = s.handleCommand(&sb, bufio.NewReader(strings.NewReader("get test\r\n")))
			require.NoError(t, err)
			want = "VALUE test 0 5\r\nhello\r\nEND\r\n"
			got = sb.String()
			require.Equal(t, want, got)
		})
	})

	t.Run("positive", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			s := newServer(testLogger(t))

			ttl := 20 * time.Second

			var sb strings.Builder
			err := s.handleCommand(&sb, bufio.NewReader(strings.NewReader("set test 0 20 5\r\nhello\r\n")))
			require.NoError(t, err)
			want := "STORED\r\n"
			got := sb.String()
			require.Equal(t, want, got)

			sb.Reset()
			err = s.handleCommand(&sb, bufio.NewReader(strings.NewReader("get test\r\n")))
			require.NoError(t, err)
			want = "VALUE test 0 5\r\nhello\r\nEND\r\n"
			got = sb.String()
			require.Equal(t, want, got)

			time.Sleep(ttl - time.Nanosecond)

			sb.Reset()
			err = s.handleCommand(&sb, bufio.NewReader(strings.NewReader("get test\r\n")))
			require.NoError(t, err)
			want = "VALUE test 0 5\r\nhello\r\nEND\r\n"
			got = sb.String()
			require.Equal(t, want, got)

			time.Sleep(time.Nanosecond)

			sb.Reset()
			err = s.handleCommand(&sb, bufio.NewReader(strings.NewReader("get test\r\n")))
			require.NoError(t, err)
			want = "END\r\n"
			got = sb.String()
			require.Equal(t, want, got)
		})
	})
}

func TestAdd(t *testing.T) {
	t.Run("item does not exist", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "add test 0 0 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
	})

	t.Run("item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "add test 0 0 9\r\nnew value\r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 9\r\nold value\r\nEND\r\n")
	})

	t.Run("expired item", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "add test 0 0 9\r\nnew value\r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 9\r\nnew value\r\nEND\r\n")
	})

	t.Run("noreply: item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "add test 0 0 9 noreply\r\nnew value\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 9\r\nold value\r\nEND\r\n")
	})

	t.Run("noreply: item does not exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "add test 0 -1 5 noreply\r\nhello\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})
}

func TestReplace(t *testing.T) {
	t.Run("item does not exist", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "replace test 0 0 5\r\nhello\r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})

	t.Run("item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "replace test 0 0 9\r\nnew value\r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 9\r\nnew value\r\nEND\r\n")
	})

	t.Run("expired item", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "replace test 0 0 9\r\nnew value\r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})

	t.Run("noreply: item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "replace test 0 0 9 noreply\r\nnew value\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 9\r\nnew value\r\nEND\r\n")
	})

	t.Run("noreply: item does not exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 9\r\nold value\r\n")
		tc.requireStored(t)

		tc.send(t, "replace test 0 0 9 noreply\r\nnew value\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})
}

func TestAppend(t *testing.T) {
	t.Run("item does not exist", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "append test 0 0 5\r\nhello\r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})

	t.Run("item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "append test 0 0 6\r\n world\r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 11\r\nhello world\r\nEND\r\n")
	})

	t.Run("expired item", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "append test 0 0 6\r\n world\r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})

	t.Run("noreply: item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "append test 0 0 6 noreply\r\n world\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 11\r\nhello world\r\nEND\r\n")
	})

	t.Run("noreply: item does not exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "append test 0 0 6 noreply\r\n world\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})
}

func TestPrepend(t *testing.T) {
	t.Run("item does not exist", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "prepend test 0 0 5\r\nhello\r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})

	t.Run("item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5\r\nworld\r\n")
		tc.requireStored(t)

		tc.send(t, "prepend test 0 0 6\r\nhello \r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 11\r\nhello world\r\nEND\r\n")
	})

	t.Run("expired item", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 5\r\nworld\r\n")
		tc.requireStored(t)

		tc.send(t, "prepend test 0 0 6\r\nhello \r\n")
		tc.requireNotStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})

	t.Run("noreply: item exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5\r\nworld\r\n")
		tc.requireStored(t)

		tc.send(t, "prepend test 0 0 6 noreply\r\nhello \r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 11\r\nhello world\r\nEND\r\n")
	})

	t.Run("noreply: item does not exists", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "prepend test 0 0 6 noreply\r\n world\r\n")
		tc.requireNoResponse(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
	})
}

func TestValueExpirationBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := time.Hour

		v := value{expiredAt: time.Now().Add(ttl)}

		time.Sleep(ttl - time.Nanosecond)

		require.False(t, v.isExpired(), "value should remain live immediately before its expiration deadline")

		time.Sleep(time.Nanosecond)

		require.True(t, v.isExpired(), "value should be expired exactly at its expiration deadline")
	})
}
