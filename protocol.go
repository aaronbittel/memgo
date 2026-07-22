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
	key       string
	flags     uint16
	exptime   int64
	dataLen   int
	omitReply bool
}

var errInvalidCommandLine = errors.New("invalid command line")

// readCommandLine reads a CRLF-terminated command line and returns it without the
// trailing CRLF. The returned byte slice is only valid till the next read call of the
// bufio.Reader.
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

// parseCmdKind parses the command kind at the beginning of commandLine and returns the
// remaining bytes.
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

// parseStoreCommandLine parses commandLine into a storeCommand.
func parseStoreCommandLine(commandLine []byte) (storeCommand, error) {
	keyBytes, commandLine, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return storeCommand{}, errors.New("missing ' ' after key")
	}
	if err := validateKey(keyBytes); err != nil {
		return storeCommand{}, fmt.Errorf("invalid key: %w", err)
	}

	flagsBytes, commandLine, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return storeCommand{}, errors.New("missing ' ' after flags")
	}
	flagU64, err := strconv.ParseUint(string(flagsBytes), 10, 16)
	if err != nil {
		return storeCommand{}, fmt.Errorf("could not parse flags: %q", flagsBytes)
	}

	exptimeBytes, commandLine, found := bytes.Cut(commandLine, []byte{' '})
	if !found {
		return storeCommand{}, errors.New("missing ' ' after expiration time")
	}
	exptime, err := strconv.ParseInt(string(exptimeBytes), 10, 64)
	if err != nil {
		return storeCommand{}, errors.New("expiration time in seconds must be an integer")
	}

	dataLenBytes, commandLine, expectingNoReplyFlag := bytes.Cut(commandLine, []byte{' '})
	dataLen, err := strconv.Atoi(string(dataLenBytes))
	if err != nil {
		return storeCommand{}, errors.New("data length must be an integer")
	}

	if err := validateDataLen(dataLen); err != nil {
		return storeCommand{}, fmt.Errorf("invalid data length: %w", err)
	}

	var omitReply bool
	if expectingNoReplyFlag {
		if !bytes.Equal(commandLine, []byte("noreply")) {
			return storeCommand{}, fmt.Errorf("expecting 'noreply' got %q", commandLine)
		}
		omitReply = true
	}

	return storeCommand{
		key:       string(keyBytes),
		flags:     uint16(flagU64),
		exptime:   exptime,
		dataLen:   dataLen,
		omitReply: omitReply,
	}, nil
}

// readDataBlock reads a data block of dataLen bytes followed by CRLF.
// It returns the data without the trailing CRLF.
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

// parseCommandKind returns the commandKind represented by commandName.
func parseCommandKind(commandName []byte) (commandKind, error) {
	switch {
	case bytes.Equal(commandName, []byte(commandSet)):
		return commandSet, nil
	case bytes.Equal(commandName, []byte(commandGet)):
		return commandGet, nil
	case bytes.Equal(commandName, []byte(commandAdd)):
		return commandAdd, nil
	case bytes.Equal(commandName, []byte(commandReplace)):
		return commandReplace, nil
	case bytes.Equal(commandName, []byte(commandAppend)):
		return commandAppend, nil
	case bytes.Equal(commandName, []byte(commandPrepend)):
		return commandPrepend, nil
	default:
		return "", fmt.Errorf("invalid command name %q", commandName)
	}
}

const maxKeyLengthInBytes = 250

// validateKey reports whether key is valid for the memcached text protocol.
func validateKey(key []byte) error {
	if len(key) == 0 {
		return errors.New("key must not be empty")
	}

	if len(key) > maxKeyLengthInBytes {
		return errors.New("key length exceeds maximum")
	}

	for _, b := range key {
		if b <= ' ' || b == 0x7f {
			return fmt.Errorf("invalid byte %q in key", b)
		}
	}

	return nil
}

// validateDataLen reports whether data length is valid for the memcached text protocol.
func validateDataLen(dataLen int) error {
	if dataLen < 0 {
		return errors.New("data length must not be negative")
	}

	if dataLen > maxValueSize {
		return fmt.Errorf("value exceeds maxValueSize %d", maxValueSize)
	}

	return nil
}
