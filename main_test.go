package main

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSettingValue(t *testing.T) {
	const addr = ":9999"

	server := server{
		logger: slog.New(slog.DiscardHandler),
		store:  make(map[string]value),
	}

	go server.ListenAndServe(addr)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	conn.Write([]byte("set test 0 0 4\r\n1234\r\n"))

	time.Sleep(50 * time.Millisecond)

	want := value{data: []byte("1234")}

	got, ok := server.store["test"]
	require.True(t, ok, "expected store to contain key 'test'")
	require.Equal(t, want, got)

	wantResponse := "STORED\r\n"
	buf := make([]byte, len(wantResponse))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, wantResponse, string(buf))
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
