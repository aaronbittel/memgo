package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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

	if err := s.store.set(cmd.key, val); err != nil {
		return err
	}

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

	added, err := s.store.add(cmd.key, val)
	if err != nil {
		return err
	}

	var resp string
	if added {
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

	replaced, err := s.store.replace(cmd.key, val)
	if err != nil {
		return err
	}

	var resp string
	if replaced {
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

	appended, err := s.store.append(cmd.key, data)
	if err != nil {
		return err
	}

	var resp string
	if appended {
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

	prepended, err := s.store.prepend(cmd.key, data)
	if err != nil {
		return err
	}

	var resp string
	if prepended {
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

func (s *server) calculateExpiryTime(expireTimeSec int) time.Time {
	if expireTimeSec == 0 {
		return time.Time{}
	}

	return time.Now().Add(time.Duration(expireTimeSec) * time.Second)
}
