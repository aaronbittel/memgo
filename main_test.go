package main

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestSetExpiry(t *testing.T) {
	t.Run("negative", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 -1 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)

		ts.requireKeyMissing(t, "test")
	})

	t.Run("zero", func(t *testing.T) {
		ts := newTestServer(t)
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 0 5\r\nhello\r\n")
		tc.requireStored(t)

		want := value{data: []byte("hello")}

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
		ts.requireStoredValue(t, "test", want)

		ts.now = func() time.Time {
			return fixedNow().AddDate(10, 0, 0)
		}

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
		ts.requireStoredValue(t, "test", want)
	})

	t.Run("positive", func(t *testing.T) {
		ts := newTestServer(t)
		clock := newFakeClock(fixedNow())
		ts.now = clock.Now

		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 20 5\r\nhello\r\n")
		tc.requireStored(t)

		want := value{data: []byte("hello"), expiredAt: clock.Now().Add(20 * time.Second)}

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
		ts.requireStoredValue(t, "test", want)

		clock.Advance(5 * time.Second)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
		ts.requireStoredValue(t, "test", want)

		clock.Advance(20 * time.Second)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)

		ts.requireKeyMissing(t, "test")
	})

	t.Run("lazy removal", func(t *testing.T) {
		ts := newTestServer(t)
		clock := newFakeClock(fixedNow())
		ts.now = clock.Now
		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "set test 0 10 5\r\nhello\r\n")
		tc.requireStored(t)

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")

		clock.Advance(20 * time.Second)

		want := value{
			data:      []byte("hello"),
			expiredAt: fixedNow().Add(10 * time.Second),
		}
		ts.requireStoredValue(t, "test", want)

		tc.send(t, "get test\r\n")
		tc.requireEnd(t)
		ts.requireKeyMissing(t, "test")
	})
}

var fixedNow = func() time.Time {
	return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
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
			name:    "stores expiry time",
			command: "set test 0 20 4\r\n1234\r\n",
			want: value{
				data:      []byte("1234"),
				expiredAt: fixedNow().Add(20 * time.Second),
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
			ts.now = fixedNow

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

func TestGet(t *testing.T) {
	t.Run("value", func(t *testing.T) {
		ts := newTestServer(t)
		ts.store.set("test", value{data: []byte("hello, world!")})

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
		ts.store.set("test", value{data: []byte("hello\r\nworld")})

		ts.serve(t)

		tc := newTestClient(t, ts.addr())

		tc.send(t, "get test\r\n")
		tc.requireResponse(t, "VALUE test 0 12\r\nhello\r\nworld\r\nEND\r\n")
	})
}

func TestParseStoreCommandLine(t *testing.T) {
	validTests := []struct {
		name  string
		input string
		want  storeCommand
	}{
		{
			name:  "valid set command",
			input: "test 0 0 4",
			want: storeCommand{
				key:     "test",
				dataLen: 4,
			},
		},
		{
			name:  "command with no reply set",
			input: "test 127 5124 4 noreply",
			want: storeCommand{
				key:           "test",
				flags:         127,
				expireTimeSec: 5124,
				dataLen:       4,
				omitReply:     true,
			},
		},
		{
			name:  "empty data",
			input: "test 0 0 0",
			want:  storeCommand{key: "test"},
		},
	}

	for _, tt := range validTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStoreCommandLine([]byte(tt.input))
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
