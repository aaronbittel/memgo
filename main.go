package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
)

const DefaultPort = 11211

type server struct {
	logger *slog.Logger

	store *concurrentMap[string, value]
}

func main() {
	port := flag.Int("port", DefaultPort, "specify port")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := &server{
		logger: logger,
		store:  newConcurrentMap[string, value](),
	}

	addr := fmt.Sprintf(":%d", *port)

	if err := server.ListenAndServe(addr); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}

	logger.Info("server closed")
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
	defer ln.Close()

	return s.Serve(ln)
}

func (s *server) handleConnection(conn net.Conn) error {
	defer conn.Close()

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

func (s *server) handleCommand(conn net.Conn, br *bufio.Reader) error {
	commandLine, err := readCommandLine(br)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidCommandLine, err)
	}

	kind, commandLine, err := parseCmdKind(commandLine)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidCommandLine, err)
	}

	switch kind {
	case commandSet:
		cmd, err := parseSetCommandLine(commandLine)
		if err != nil {
			return err
		}
		return s.handleSet(conn, br, cmd)
	case commandGet:
		return s.handleGet(conn, commandLine)
	default:
		return nil
	}
}

func (s *server) handleGet(conn net.Conn, key []byte) error {
	var buf bytes.Buffer

	if val, ok := s.store.get(string(key)); ok {
		fmt.Fprintf(&buf, "VALUE %s %d %d\r\n%s\r\n", key, val.flags, len(val.data), val.data)
	}

	buf.WriteString("END\r\n")
	_, err := buf.WriteTo(conn)
	return err
}

func readCommandLine(br *bufio.Reader) ([]byte, error) {
	commandLine, err := br.ReadSlice('\n')
	if err != nil {
		switch {
		case errors.Is(err, io.EOF) && len(commandLine) == 0:
			return nil, io.EOF
		default:
			return nil, fmt.Errorf("incomplete command line: %w", err)
		}
	}

	if !bytes.HasSuffix(commandLine, []byte("\r\n")) {
		return nil, errors.New("command line missing \"\\r\\n\"")
	}
	commandLine = commandLine[:len(commandLine)-2]
	return commandLine, nil
}

func (s *server) handleSet(conn net.Conn, br *bufio.Reader, cmd setCommand) error {
	dataWithCrlf := make([]byte, cmd.dataLen+2) // crlf
	if _, err := io.ReadFull(br, dataWithCrlf); err != nil {
		return err
	}
	if !bytes.HasSuffix(dataWithCrlf, []byte("\r\n")) {
		return errors.New("set data block missing \"\\r\\n\"")
	}
	data := dataWithCrlf[:len(dataWithCrlf)-2]

	val := value{
		data:          data,
		flags:         cmd.flags,
		expireTimeSec: cmd.expireTimeSec,
	}

	s.store.set(cmd.key, val)

	conn.Write([]byte("STORED\r\n"))
	return nil
}

type commandKind string

const (
	commandSet commandKind = "set"
	commandGet commandKind = "get"
)

type setCommand struct {
	key           string
	flags         uint16
	expireTimeSec int
	dataLen       int
	omitReply     bool
}

type value struct {
	data          []byte
	flags         uint16
	expireTimeSec int
}

var errInvalidCommandLine = errors.New("invalid command line")

func parseCmdKind(commandLine []byte) (kind commandKind, rest []byte, err error) {
	name, rest, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return "", nil, errors.New("missing ' ' after command name")
	}
	kind, err = parseCommandKind(name)
	if err != nil {
		return "", nil, err
	}

	return kind, rest, nil
}

func parseSetCommandLine(commandLine []byte) (setCommand, error) {
	key, rest, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return setCommand{}, errors.New("missing ' ' after key")
	}

	flagsBytes, rest, found := bytes.Cut(rest, []byte{' '})
	if !found {
		return setCommand{}, errors.New("missing ' ' after flags")
	}
	flagU64, err := strconv.ParseUint(string(flagsBytes), 10, 16)
	if err != nil {
		return setCommand{}, fmt.Errorf("could not parse flags: %q", flagsBytes)
	}

	expireTimeBytes, rest, found := bytes.Cut(rest, []byte{' '})
	if !found {
		return setCommand{}, errors.New("missing ' ' after expiration time")
	}
	expireTimeSec, err := strconv.Atoi(string(expireTimeBytes))
	if err != nil {
		return setCommand{}, errors.New("expiration time in seconds must be an interger")
	}

	dataLenBytes, rest, expectingNoReplyFlag := bytes.Cut(rest, []byte{' '})
	dataLen, err := strconv.Atoi(string(dataLenBytes))
	if err != nil {
		return setCommand{}, errors.New("data length must be an interger")
	}

	var omitReply bool
	if expectingNoReplyFlag {
		if !bytes.Equal(rest, []byte("noreply")) {
			return setCommand{}, fmt.Errorf("expecting 'noreply' got %q", rest)
		}
		omitReply = true
	}

	return setCommand{
		key:           string(key),
		flags:         uint16(flagU64),
		expireTimeSec: expireTimeSec,
		dataLen:       dataLen,
		omitReply:     omitReply,
	}, nil
}

func parseCommandKind(name []byte) (commandKind, error) {
	switch {
	case bytes.Equal(name, []byte(commandSet)):
		return commandSet, nil
	case bytes.Equal(name, []byte(commandGet)):
		return commandGet, nil
	default:
		return "", fmt.Errorf("invalid command name %q", name)
	}
}
