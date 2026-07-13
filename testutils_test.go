package main

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestListener(t *testing.T) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		ln.Close()
	})

	return ln
}

type testClient struct {
	conn net.Conn
}

func newTestClient(t *testing.T, addr string) *testClient {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close()
	})

	return &testClient{conn: conn}
}

func (tc *testClient) send(t *testing.T, s string) {
	t.Helper()

	require.NoError(t, tc.conn.SetWriteDeadline(time.Now().Add(time.Second)))

	_, err := io.WriteString(tc.conn, s)
	require.NoError(t, err)
}

func (tc *testClient) assertReadEquals(t *testing.T, want string) {
	t.Helper()

	require.NoError(t, tc.conn.SetReadDeadline(time.Now().Add(time.Second)))

	buf := make([]byte, len(want))
	n, err := io.ReadFull(tc.conn, buf)
	require.NoErrorf(t, err, "\nexpected:\n%q\ngot:\n%q", want, buf[:n])
	require.Equal(t, want, string(buf))
}

type testServer struct {
	server
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

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	return &testServer{
		server: server{
			logger: slog.New(slog.DiscardHandler),
			store:  newConcurrentMap[string, value](),
		},
		ln:       newTestListener(t),
		serveErr: make(chan error, 1),
	}
}

type testClientE struct {
	conn net.Conn
}

func newTestClientE(addr string) (*testClientE, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &testClientE{conn: conn}, nil
}

func (tce *testClientE) send(s string) error {
	if err := tce.conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}

	_, err := io.WriteString(tce.conn, s)
	return err
}
