package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

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

var (
	crlf      = []byte("\r\n")
	end       = []byte("END\r\n")
	stored    = []byte("STORED\r\n")
	notStored = []byte("NOT_STORED\r\n")
	valueResp = []byte("VALUE")
)

func (s *server) handleGet(w io.Writer, key []byte) error {
	val, ok := s.store.get(string(key))
	if !ok {
		_, err := w.Write(end)
		return err
	}

	header := make([]byte, 0, len(key)+48)
	header = append(header, valueResp...)
	header = append(header, ' ')
	header = append(header, key...)
	header = append(header, ' ')
	header = strconv.AppendUint(header, uint64(val.flags), 10)
	header = append(header, ' ')
	header = strconv.AppendInt(header, int64(len(val.data)), 10)
	header = append(header, '\r', '\n')

	bufs := net.Buffers{header, val.data, crlf, end}

	_, err := bufs.WriteTo(w)
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

	if err := s.store.set(cmd.key, val); err != nil {
		return err
	}

	if !cmd.omitReply {
		if _, err := w.Write(stored); err != nil {
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

	added, err := s.store.add(cmd.key, val)
	if err != nil {
		return err
	}

	if cmd.omitReply {
		return nil
	}

	var resp []byte
	if added {
		resp = stored
	} else {
		resp = notStored
	}

	_, err = w.Write(resp)
	return err
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

	replaced, err := s.store.replace(cmd.key, val)
	if err != nil {
		return err
	}

	if cmd.omitReply {
		return nil
	}

	var resp []byte
	if replaced {
		resp = stored
	} else {
		resp = notStored
	}

	_, err = w.Write(resp)
	return err
}

func (s *server) handleAppend(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	appended, err := s.store.append(cmd.key, data)
	if err != nil {
		return err
	}

	if cmd.omitReply {
		return nil
	}

	var resp []byte
	if appended {
		resp = stored
	} else {
		resp = notStored
	}

	_, err = w.Write(resp)
	return err
}

func (s *server) handlePrepend(w io.Writer, br *bufio.Reader, cmd storeCommand) error {
	data, err := readDataBlock(br, cmd.dataLen)
	if err != nil {
		return err
	}

	prepended, err := s.store.prepend(cmd.key, data)
	if err != nil {
		return err
	}

	if cmd.omitReply {
		return nil
	}

	var resp []byte
	if prepended {
		resp = stored
	} else {
		resp = notStored
	}

	_, err = w.Write(resp)
	return err
}

func (s *server) calculateExpiryTime(expireTimeSec int) time.Time {
	if expireTimeSec == 0 {
		return time.Time{}
	}

	return time.Now().Add(time.Duration(expireTimeSec) * time.Second)
}
