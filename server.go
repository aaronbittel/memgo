package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
)

const DefaultPort = 11211

type server struct {
	logger *slog.Logger

	store *store
}

func newServer(logger *slog.Logger) *server {
	return &server{
		logger: logger,
		store:  newStore(),
	}
}

func (s *server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.logger.Error("accepting connection", "err", err)
			return err
		}

		s.logger.Info("connection received", "addr", conn.RemoteAddr())

		go func() {
			if err := s.handleConnection(conn); err != nil {
				s.logger.Error("handling connection", "remote_addr", conn.RemoteAddr(), "err", err)
			}
		}()
	}
}

func (s *server) ListenAndServe(addr string) error {
	s.logger.Info("server starting", "addr", addr)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.logger.Error("listening", "addr", addr, "err", err)
		return err
	}
	defer func() {
		if err := ln.Close(); err != nil {
			s.logger.Error("closing listener", "addr", addr, "err", err)
		}
	}()

	return s.Serve(ln)
}

func (s *server) handleConnection(conn net.Conn) error {
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Error("closing connection", "conn", conn.RemoteAddr(), "err", err)
		}
	}()

	var (
		br  = bufio.NewReader(conn)
		err error
	)

	for {
		if err = s.handleCommand(conn, br); err != nil {
			switch {
			case errors.Is(err, io.EOF):
				s.logger.Info("client disconnected", "addr", conn.RemoteAddr())
				return nil
			default:
				return fmt.Errorf("handle command: %v", err)
			}
		}
	}
}
