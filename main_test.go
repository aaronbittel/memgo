package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSetAndGet(t *testing.T) {
	ts := newTestServer(t)
	ts.serve(t)

	tc := newTestClient(t, ts.addr())

	tc.send(t, "get test\r\n")
	tc.assertReadEquals(t, "END\r\n")

	tc.send(t, "set test 0 0 5\r\nhello\r\n")
	tc.assertReadEquals(t, "STORED\r\n")

	tc.send(t, "get test\r\n")
	tc.assertReadEquals(t, "VALUE test 0 5\r\nhello\r\nEND\r\n")
}

func TestServerSettingValue(t *testing.T) {
	ts := newTestServer(t)
	ts.serve(t)

	tc := newTestClient(t, ts.addr())

	tc.send(t, "set test 0 0 4\r\n1234\r\n")
	tc.assertReadEquals(t, "STORED\r\n")

	want := value{data: []byte("1234")}
	got, ok := ts.store["test"]
	require.True(t, ok, "expected store to contain key 'test'")
	require.Equal(t, want, got)
}

func TestServerRetrievingValue(t *testing.T) {
	ts := newTestServer(t)
	ts.store["test"] = value{data: []byte("hello, world!")}

	ts.serve(t)

	tc := newTestClient(t, ts.addr())

	tc.send(t, "get test\r\n")
	tc.assertReadEquals(t, "VALUE test 0 13\r\nhello, world!\r\nEND\r\n")
}

func TestServerGetMissingValue(t *testing.T) {
	ts := newTestServer(t)
	ts.serve(t)

	tc := newTestClient(t, ts.addr())

	tc.send(t, "get missing\r\n")
	tc.assertReadEquals(t, "END\r\n")
}

func TestParseSetCommandLine(t *testing.T) {
	validTests := []struct {
		name  string
		input string
		want  setCommand
	}{
		{
			name:  "valid set command",
			input: "test 0 0 4",
			want: setCommand{
				key:     "test",
				dataLen: 4,
			},
		},
		{
			name:  "command with no reply set",
			input: "test 127 5124 4 noreply",
			want: setCommand{
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
			want:  setCommand{key: "test"},
		},
	}

	for _, tt := range validTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSetCommandLine([]byte(tt.input))
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
