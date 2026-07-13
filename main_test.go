package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSettingValue(t *testing.T) {
	const port = 9999

	server := server{
		logger: slog.New(slog.DiscardHandler),
		port:   port,
		store:  make(map[string]value),
	}

	go server.Start()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
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

func TestParseCommand(t *testing.T) {
	input := []byte("set test 0 0 4\r\n1234\r\n")

	wantKey := "test"
	wantValue := value{
		data:          []byte("1234"),
		flags:         0,
		expireTimeSec: 0,
	}

	br := bufio.NewReader(bytes.NewReader(input))
	key, value, err := parseCommand(br)
	require.NoError(t, err)

	assert.Equal(t, wantKey, key)
	assert.Equal(t, wantValue, value)
}

func TestParseCommandLine(t *testing.T) {
	validTests := []struct {
		name  string
		input string
		want  *command
	}{
		{
			name:  "valid set command",
			input: "set test 0 0 4\r\n",
			want: &command{
				kind:    commandSet,
				key:     "test",
				dataLen: 4,
			},
		},
		{
			name:  "command with no reply set",
			input: "set test 127 5124 4 noreply\r\n",
			want: &command{
				kind:          commandSet,
				key:           "test",
				flags:         127,
				expireTimeSec: 5124,
				dataLen:       4,
				omitReplay:    true,
			},
		},
		{
			name:  "empty data",
			input: "set test 0 0 0\r\n",
			want: &command{
				kind: commandSet,
				key:  "test",
			},
		},
	}

	for _, tt := range validTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommandLine([]byte(tt.input))
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
