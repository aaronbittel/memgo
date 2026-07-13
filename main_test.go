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
	server := server{
		logger: slog.New(slog.DiscardHandler),
		store:  make(map[string]value),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- server.Serve(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	defer conn.Close()

	_, err = io.WriteString(conn, "set test 0 0 4\r\n1234\r\n")
	require.NoError(t, err)

	wantResponse := "STORED\r\n"
	buf := make([]byte, len(wantResponse))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, wantResponse, string(buf))

	want := value{data: []byte("1234")}
	got, ok := server.store["test"]
	require.True(t, ok, "expected store to contain key 'test'")
	require.Equal(t, want, got)

	require.NoError(t, <-serveErr)
}

func TestServerRetrievingValue(t *testing.T) {
	server := server{
		logger: slog.New(slog.DiscardHandler),
		store: map[string]value{
			"test": value{data: []byte("hello, world!")},
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	_, err = io.WriteString(conn, "get test\r\n")
	require.NoError(t, err)

	want := "VALUE hello, world! 0 13\r\n"
	buf := make([]byte, len(want))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)

	assert.Equal(t, want, string(buf))

	require.NoError(t, <-serveErr)
}

func TestServerGetMissingValue(t *testing.T) {
	server := server{
		logger: slog.New(slog.DiscardHandler),
		store:  make(map[string]value),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	_, err = io.WriteString(conn, "get missing\r\n")
	require.NoError(t, err)

	want := "END\r\n"
	buf := make([]byte, len(want))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)

	assert.Equal(t, want, string(buf))

	require.NoError(t, <-serveErr)
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
