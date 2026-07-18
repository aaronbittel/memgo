package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestListener(t *testing.T) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = ln.Close()
	})

	return ln
}

type testClient struct {
	conn net.Conn
	r    *bufio.Reader
}

func newTestClient(t *testing.T, addr string) *testClient {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
	})

	return &testClient{conn: conn, r: bufio.NewReader(conn)}
}

func (tc *testClient) send(t *testing.T, s string) {
	t.Helper()

	require.NoError(t, tc.conn.SetWriteDeadline(time.Now().Add(time.Second)))
	defer func() {
		_ = tc.conn.SetWriteDeadline(time.Time{})
	}()

	_, err := io.WriteString(tc.conn, s)
	require.NoError(t, err)
}

func (tc *testClient) requireStored(t *testing.T) {
	t.Helper()

	tc.requireLine(t, "STORED\r\n")
}

func (tc *testClient) requireNotStored(t *testing.T) {
	t.Helper()

	tc.requireLine(t, "NOT_STORED\r\n")
}

func (tc *testClient) requireEnd(t *testing.T) {
	t.Helper()

	tc.requireLine(t, "END\r\n")
}

func (tc *testClient) requireLine(t *testing.T, want string) {
	t.Helper()

	got := tc.readLine(t)

	require.Equal(t, want, got)
}

func (tc *testClient) readLine(t *testing.T) string {
	t.Helper()

	line, err := tc.r.ReadString('\n')
	require.NoErrorf(t, err, "reading response line; partial line: %q", line)

	return line
}

func (tc *testClient) requireResponse(t *testing.T, want string) {
	t.Helper()

	if !strings.HasSuffix(want, "\r\n") {
		t.Fatalf("requireLine: want must contain one CRLF-terminated line, got %q", want)
	}
	require.NoError(t, tc.conn.SetReadDeadline(time.Now().Add(time.Second)))
	defer func() {
		_ = tc.conn.SetReadDeadline(time.Time{})
	}()

	var sb strings.Builder
	for range strings.Count(want, "\n") {
		line := tc.readLine(t)
		sb.WriteString(line)
	}

	got := sb.String()
	require.Equal(t, want, got)
}

func (tc *testClient) requireNoResponse(t *testing.T) {
	t.Helper()

	require.NoError(t, tc.conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	defer func() {
		_ = tc.conn.SetReadDeadline(time.Time{})
	}()

	_, err := tc.r.Peek(1)

	if err == nil {
		n := tc.r.Buffered()
		got, _ := tc.r.Peek(n)
		t.Fatalf("unexpected response: %q", got)
	}

	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

type testServer struct {
	*server
	ln       net.Listener
	serveErr chan error
}

func (ts *testServer) addr() string {
	return ts.ln.Addr().String()
}

func (ts *testServer) serve(t *testing.T) {
	t.Helper()

	go func() {
		ts.serveErr <- ts.Serve(ts.ln)
	}()

	t.Cleanup(func() {
		if err := ts.ln.Close(); err != nil {
			t.Errorf("close server listener: %v", err)
		}

		select {
		case err := <-ts.serveErr:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Errorf("server exited with error: %v", err)
			}
		case <-time.After(time.Second):
			t.Errorf("server did not stop within timeout")
		}
	})
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()

	return slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{
		Level:     slog.LevelError,
		AddSource: true,
	}))
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	return &testServer{
		server:   newServer(testLogger(t)),
		ln:       newTestListener(t),
		serveErr: make(chan error, 1),
	}
}

func (ts *testServer) requireStoredValue(t *testing.T, key string, want value) {
	t.Helper()

	ts.store.mu.Lock()
	got, ok := ts.store.store[key]
	ts.store.mu.Unlock()
	require.True(t, ok, "expected store to contain key 'test'")
	require.Equal(t, want, got)
}

type testClientE struct {
	conn net.Conn
	r    *bufio.Reader
}

func newTestClientE(addr string) (*testClientE, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &testClientE{
		conn: conn,
		r:    bufio.NewReader(conn),
	}, nil
}

func (tce *testClientE) send(s string) error {
	if err := tce.conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	defer func() {
		_ = tce.conn.SetWriteDeadline(time.Time{})
	}()

	_, err := io.WriteString(tce.conn, s)
	return err
}

func (tce *testClientE) recv(want string) (string, error) {
	if err := tce.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return "", err
	}
	defer func() {
		_ = tce.conn.SetReadDeadline(time.Time{})
	}()

	var sb strings.Builder
	for range strings.Count(want, "\n") {
		line, err := tce.r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading failed: %v", err)
		}
		sb.WriteString(line)
	}

	return sb.String(), nil
}
