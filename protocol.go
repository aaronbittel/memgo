package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
)

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

var errInvalidCommandLine = errors.New("invalid command line")

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
