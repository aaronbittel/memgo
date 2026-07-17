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
	"time"
)

// TODO: use !now.Before(v.expiredAt) to expire an item at the expire time

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

func main() {
	port := flag.Int("port", DefaultPort, "specify port")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := newServer(logger)

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

func (s *server) handleCommand(w io.Writer, br *bufio.Reader) error {
	commandLine, err := readCommandLine(br)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidCommandLine, err)
	}

	kind, commandLine, err := parseCmdKind(commandLine)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidCommandLine, err)
	}

	return s.dispatchCommand(w, br, kind, commandLine)
}

func (s *server) dispatchCommand(w io.Writer, br *bufio.Reader, kind commandKind, commandLine []byte) error {
	switch kind {
	case commandGet:
		return s.handleGet(w, commandLine)
	case commandSet:
		cmd, err := parseStoreCommandLine(commandLine)
		if err != nil {
			return err
		}
		return s.handleSet(w, br, cmd)
	case commandAdd:
		cmd, err := parseStoreCommandLine(commandLine)
		if err != nil {
			return err
		}
		return s.handleAdd(w, br, cmd)
	case commandReplace:
		cmd, err := parseStoreCommandLine(commandLine)
		if err != nil {
			return err
		}
		return s.handleReplace(w, br, cmd)
	case commandAppend:
		cmd, err := parseStoreCommandLine(commandLine)
		if err != nil {
			return err
		}
		return s.handleAppend(w, br, cmd)
	case commandPrepend:
		cmd, err := parseStoreCommandLine(commandLine)
		if err != nil {
			return err
		}
		return s.handlePrepend(w, br, cmd)
	default:
		return fmt.Errorf("illegal kind %q", kind)
	}
}

func (s *server) handleGet(w io.Writer, key []byte) error {
	var buf bytes.Buffer

	val, ok := s.store.get(string(key))
	if ok {
		fmt.Fprintf(&buf, "VALUE %s %d %d\r\n%s\r\n", key, val.flags, len(val.data), val.data)
	}
	buf.WriteString("END\r\n")

	_, err := buf.WriteTo(w)
	return err
}

func (s *server) handleSet(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	val := value{
		data:      data,
		flags:     cmd.flags,
		expiredAt: s.calculateExpiryTime(cmd.expireTimeSec),
	}

	s.store.set(cmd.key, val)

	if !cmd.omitReply {
		if _, err := io.WriteString(w, "STORED\r\n"); err != nil {
			return err
		}
	}

	return nil
}

func (s *server) handleAdd(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	val := value{
		data:      data,
		flags:     cmd.flags,
		expiredAt: s.calculateExpiryTime(cmd.expireTimeSec),
	}

	var resp string

	if s.store.add(cmd.key, val) {
		resp = "STORED\r\n"
	} else {
		resp = "NOT_STORED\r\n"
	}

	if !cmd.omitReply {
		if _, err := io.WriteString(w, resp); err != nil {
			return err
		}
	}

	return nil
}

func (s *server) handleReplace(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	val := value{
		data:      data,
		flags:     cmd.flags,
		expiredAt: s.calculateExpiryTime(cmd.expireTimeSec),
	}

	var resp string
	if s.store.replace(cmd.key, val) {
		resp = "STORED\r\n"
	} else {
		resp = "NOT_STORED\r\n"
	}

	if !cmd.omitReply {
		if _, err := io.WriteString(w, resp); err != nil {
			return err
		}
	}

	return nil
}

func (s *server) handleAppend(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	var resp string
	if s.store.append(cmd.key, data) {
		resp = "STORED\r\n"
	} else {
		resp = "NOT_STORED\r\n"
	}

	if !cmd.omitReply {
		if _, err := io.WriteString(w, resp); err != nil {
			return err
		}
	}

	return nil
}

func (s *server) handlePrepend(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	var resp string
	if s.store.prepend(cmd.key, data) {
		resp = "STORED\r\n"
	} else {
		resp = "NOT_STORED\r\n"
	}

	if !cmd.omitReply {
		if _, err := io.WriteString(w, resp); err != nil {
			return err
		}
	}

	return nil
}

func readDataBlock(r io.Reader, dataLen int) ([]byte, error) {
	dataWithCrlf := make([]byte, dataLen+2) // crlf
	if _, err := io.ReadFull(r, dataWithCrlf); err != nil {
		return nil, err
	}
	if !bytes.HasSuffix(dataWithCrlf, []byte("\r\n")) {
		return nil, errors.New("data block missing \"\\r\\n\"")
	}
	return dataWithCrlf[:len(dataWithCrlf)-2], nil
}

func (s *server) calculateExpiryTime(expireTimeSec int) time.Time {
	if expireTimeSec == 0 {
		return time.Time{}
	}

	return time.Now().Add(time.Duration(expireTimeSec) * time.Second)
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

type commandKind string

const (
	commandSet     commandKind = "set"
	commandGet     commandKind = "get"
	commandAdd     commandKind = "add"
	commandReplace commandKind = "replace"
	commandAppend  commandKind = "append"
	commandPrepend commandKind = "prepend"
)

type storeCommand struct {
	key           string
	flags         uint16
	expireTimeSec int
	dataLen       int
	omitReply     bool
}

type value struct {
	data      []byte
	flags     uint16
	expiredAt time.Time
}

func (v value) isExpired(t time.Time) bool {
	if v.expiredAt.IsZero() {
		return false
	}

	return v.expiredAt.Before(t)
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

func parseStoreCommandLine(commandLine []byte) (storeCommand, error) {
	keyBytes, commandLine, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return storeCommand{}, errors.New("missing ' ' after key")
	}

	flagsBytes, commandLine, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return storeCommand{}, errors.New("missing ' ' after flags")
	}
	flagU64, err := strconv.ParseUint(string(flagsBytes), 10, 16)
	if err != nil {
		return storeCommand{}, fmt.Errorf("could not parse flags: %q", flagsBytes)
	}

	expireTimeBytes, commandLine, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return storeCommand{}, errors.New("missing ' ' after expiration time")
	}
	expireTimeSec, err := strconv.Atoi(string(expireTimeBytes))
	if err != nil {
		return storeCommand{}, errors.New("expiration time in seconds must be an interger")
	}

	dataLenBytes, commandLine, expectingNoReplyFlag := bytes.Cut(commandLine, []byte{' '})
	dataLen, err := strconv.Atoi(string(dataLenBytes))
	if err != nil {
		return storeCommand{}, errors.New("data length must be an interger")
	}

	var omitReply bool
	if expectingNoReplyFlag {
		if !bytes.Equal(commandLine, []byte("noreply")) {
			return storeCommand{}, fmt.Errorf("expecting 'noreply' got %q", commandLine)
		}
		omitReply = true
	}

	return storeCommand{
		key:           string(keyBytes),
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
	case bytes.Equal(name, []byte(commandAdd)):
		return commandAdd, nil
	case bytes.Equal(name, []byte(commandReplace)):
		return commandReplace, nil
	case bytes.Equal(name, []byte(commandAppend)):
		return commandAppend, nil
	case bytes.Equal(name, []byte(commandPrepend)):
		return commandPrepend, nil
	default:
		return "", fmt.Errorf("invalid command name %q", name)
	}
}
