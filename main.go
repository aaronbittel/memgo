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
	port   int

	store map[string]value
}

func main() {
	port := flag.Int("port", DefaultPort, "specify port")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := &server{
		logger: logger,
		port:   *port,
		store:  make(map[string]value),
	}

	server.Start()

	select {}
}

func (server *server) Start() error {
	server.logger.Info("server starting", "port", server.port)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", server.port))
	if err != nil {
		server.logger.Error("listening", "port", server.port, "err", err)
		return err
	}
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		server.logger.Error("accepting connection", "err", err)
		return err
	}

	server.logger.Info("connection received", "addr", conn.RemoteAddr())

	go server.handleConnection(conn)

	return nil
}

func (server *server) handleConnection(conn net.Conn) {
	defer conn.Close()

	br := bufio.NewReader(conn)
	key, value, err := parseCommand(br)
	if err != nil {
		server.logger.Error("parsing command", "err", err)
		return
	}

	server.store[key] = value
	conn.Write([]byte("STORED\r\n"))
}

type commandKind string

const (
	commandSet commandKind = "set"
)

type command struct {
	kind          commandKind
	key           string
	flags         uint16
	expireTimeSec int
	dataLen       int
	omitReplay    bool
}

type value struct {
	data          []byte
	flags         uint16
	expireTimeSec int
}

func parseCommand(br *bufio.Reader) (key string, val value, err error) {
	commandLine, err := br.ReadSlice('\n')
	if err != nil {
		return "", value{}, err
	}

	cmd, err := parseCommandLine(commandLine)
	if err != nil {
		return "", value{}, err
	}

	dataWithCrlf := make([]byte, cmd.dataLen+2) // crlf
	if _, err := io.ReadFull(br, dataWithCrlf); err != nil {
		return "", value{}, fmt.Errorf("read data block %d: %v", len(dataWithCrlf), err)
	}

	if !bytes.HasSuffix(dataWithCrlf, []byte("\r\n")) {
		return "", value{}, errors.New("missing \"\\r\\n\" after data block")
	}

	val = value{
		data:          dataWithCrlf[:len(dataWithCrlf)-2],
		flags:         cmd.flags,
		expireTimeSec: cmd.expireTimeSec,
	}

	return cmd.key, val, nil
}

var errInvalidCommandLine = errors.New("invalid command line")

func parseCommandLine(commandLine []byte) (*command, error) {
	if !bytes.HasSuffix(commandLine, []byte("\r\n")) {
		return nil, errors.New("missing \"\\r\\n\"")
	}
	commandLine = commandLine[:len(commandLine)-2]

	name, rest, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return nil, errors.New("missing ' ' after command name")
	}
	kind, err := parseCommandKind(name)
	if err != nil {
		return nil, err
	}

	key, rest, found := bytes.Cut(rest, []byte{' '})
	if !found {
		return nil, errors.New("missing ' ' after key")
	}

	flagsBytes, rest, found := bytes.Cut(rest, []byte{' '})
	if !found {
		return nil, errors.New("missing ' ' after flags")
	}
	flagU64, err := strconv.ParseUint(string(flagsBytes), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("could not parse flags: %q", flagsBytes)
	}

	expireTimeBytes, rest, found := bytes.Cut(rest, []byte{' '})
	if !found {
		return nil, errors.New("missing ' ' after expiration time")
	}
	expireTimeSec, err := strconv.Atoi(string(expireTimeBytes))
	if err != nil {
		return nil, errors.New("expiration time in seconds must be an interger")
	}

	dataLenBytes, rest, expectingNoReplyFlag := bytes.Cut(rest, []byte{' '})
	dataLen, err := strconv.Atoi(string(dataLenBytes))
	if err != nil {
		return nil, errors.New("data length must be an interger")
	}

	var omitReply bool
	if expectingNoReplyFlag {
		if !bytes.Equal(rest, []byte("noreply")) {
			return nil, fmt.Errorf("expecting 'noreply' got %q", rest)
		}
		omitReply = true
	}

	return &command{
		kind:          kind,
		key:           string(key),
		flags:         uint16(flagU64),
		expireTimeSec: expireTimeSec,
		dataLen:       dataLen,
		omitReplay:    omitReply,
	}, nil
}

func parseCommandKind(name []byte) (commandKind, error) {
	switch {
	case bytes.Equal(name, []byte(commandSet)):
		return commandSet, nil
	default:
		return "", fmt.Errorf("invalid command name %q", name)
	}
}
