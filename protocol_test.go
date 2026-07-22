package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCommandLine(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		br := bufio.NewReader(strings.NewReader("hello, world\r\n"))

		got, err := readCommandLine(br)

		require.NoError(t, err)
		assert.Equal(t, []byte("hello, world"), got)
	})

	t.Run("empty input", func(t *testing.T) {
		br := bufio.NewReader(strings.NewReader(""))

		got, err := readCommandLine(br)

		require.ErrorIs(t, err, io.EOF)
		assert.Nil(t, got)
	})

	t.Run("incomplete line", func(t *testing.T) {
		br := bufio.NewReader(strings.NewReader("hello, world\r"))

		got, err := readCommandLine(br)

		require.ErrorIs(t, err, io.EOF)
		assert.Nil(t, got)
	})

	t.Run("missing carriage return", func(t *testing.T) {
		br := bufio.NewReader(strings.NewReader("hello, world\n"))

		got, err := readCommandLine(br)

		require.EqualError(t, err, `command line missing "\r\n"`)
		assert.Nil(t, got)
	})

	t.Run("reader error", func(t *testing.T) {
		readErr := errors.New("read failed")
		br := bufio.NewReader(iotest.ErrReader(readErr))

		got, err := readCommandLine(br)

		require.ErrorIs(t, err, readErr)
		assert.ErrorContains(t, err, "incomplete command line")
		assert.Nil(t, got)
	})

	t.Run("leaves subsequent data unread", func(t *testing.T) {
		br := bufio.NewReader(strings.NewReader("hello, world\r\nand some more data"))

		want := []byte("hello, world")
		got, err := readCommandLine(br)

		require.NoError(t, err)
		assert.Equal(t, want, got)

		rest, err := io.ReadAll(br)
		require.NoError(t, err)
		require.Equal(t, []byte("and some more data"), rest)
	})

	t.Run("empty command line", func(t *testing.T) {
		br := bufio.NewReader(strings.NewReader("\r\n"))

		got, err := readCommandLine(br)

		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestParseCmdKind(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			input string
			want  commandKind
		}{
			{"set key", commandSet},
			{"get key", commandGet},
			{"add key", commandAdd},
			{"replace key", commandReplace},
			{"append key", commandAppend},
			{"prepend key", commandPrepend},
		}

		for _, tt := range tests {
			t.Run(string(tt.want), func(t *testing.T) {
				got, rest, err := parseCmdKind([]byte(tt.input))

				require.NoError(t, err)
				require.Equal(t, tt.want, got)
				require.Equal(t, []byte("key"), rest)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Run("missing space", func(t *testing.T) {
			_, _, err := parseCmdKind([]byte("commandLineMissingSpace"))
			require.Error(t, err)
			assert.ErrorContains(t, err, "missing ' ' after command name")
		})

		t.Run("kind", func(t *testing.T) {
			_, _, err := parseCmdKind([]byte("some other stuff"))
			require.Error(t, err)
			require.ErrorContains(t, err, "invalid command name")
		})

		t.Run("empty rest", func(t *testing.T) {
			kind, rest, err := parseCmdKind([]byte("set "))

			require.NoError(t, err)
			require.Equal(t, commandSet, kind)
			require.Empty(t, rest)
		})
	})

}

func TestParseStoreCommandLine(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  storeCommand
		}{
			{
				name:  "set command",
				input: "test 0 0 4",
				want: storeCommand{
					key:     "test",
					dataLen: 4,
				},
			},
			{
				name:  "command with no reply set",
				input: "test 127 5124 4 noreply",
				want: storeCommand{
					key:       "test",
					flags:     127,
					exptime:   5124,
					dataLen:   4,
					omitReply: true,
				},
			},
			{
				name:  "empty data",
				input: "test 0 0 0",
				want:  storeCommand{key: "test"},
			},
			{
				name:  "maximum flags value",
				input: "test 65535 0 1",
				want: storeCommand{
					key:     "test",
					flags:   65535,
					dataLen: 1,
				},
			},
			{
				name:  "negative expiration time",
				input: "test 0 -1 1",
				want: storeCommand{
					key:     "test",
					exptime: -1,
					dataLen: 1,
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := parseStoreCommandLine([]byte(tt.input))
				require.NoError(t, err)

				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			name    string
			input   string
			wantErr string
		}{
			{
				name:    "missing space after key",
				input:   "test",
				wantErr: "missing ' ' after key",
			},
			{
				name:    "empty key",
				input:   " 0 0 4",
				wantErr: "invalid key: key must not be empty",
			},
			{
				name:    "missing space after flags",
				input:   "test 0",
				wantErr: "missing ' ' after flags",
			},
			{
				name:    "invalid flags",
				input:   "test invalid 0 4",
				wantErr: "could not parse flags",
			},
			{
				name:    "flags exceed uint16",
				input:   "test 65536 0 4",
				wantErr: "could not parse flags",
			},
			{
				name:    "missing space after expiration time",
				input:   "test 0 0",
				wantErr: "missing ' ' after expiration time",
			},
			{
				name:    "invalid expiration time",
				input:   "test 0 invalid 4",
				wantErr: "expiration time in seconds must be an integer",
			},
			{
				name:    "missing data length",
				input:   "test 0 0 ",
				wantErr: "data length must be an integer",
			},
			{
				name:    "invalid data length",
				input:   "test 0 0 invalid",
				wantErr: "data length must be an integer",
			},
			{
				name:    "negative data length",
				input:   "test 0 0 -2",
				wantErr: "data length must not be negative",
			},
			{
				name:    "invalid optional argument",
				input:   "test 0 0 4 reply",
				wantErr: "expecting 'noreply'",
			},
			{
				name:    "extra argument after noreply",
				input:   "test 0 0 4 noreply extra",
				wantErr: "expecting 'noreply'",
			},
			{
				name:    "empty key",
				input:   " 0 0 4",
				wantErr: "key must not be empty",
			},
			{
				name:    "multiple spaces between fields",
				input:   "test  0 0 4",
				wantErr: "could not parse flags",
			},
			{
				name:    "trailing space",
				input:   "test 0 0 4 ",
				wantErr: "expecting 'noreply'",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := parseStoreCommandLine([]byte(tt.input))

				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Equal(t, storeCommand{}, got)
			})
		}
	})
}

func TestReadDataBlock(t *testing.T) {
	t.Run("err reader", func(t *testing.T) {
		readErr := errors.New("read failed")
		r := iotest.ErrReader(readErr)

		_, err := readDataBlock(r, 10)
		require.ErrorIs(t, err, readErr)
	})

	t.Run("too few bytes", func(t *testing.T) {
		r := strings.NewReader("hello world\r\n")
		_, err := readDataBlock(r, 20)

		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("too many bytes", func(t *testing.T) {
		r := strings.NewReader("hello world\r\n")
		_, err := readDataBlock(r, 2)

		assert.Error(t, err)
		assert.ErrorContains(t, err, `data block missing "\r\n"`)
	})

	t.Run("empty data block", func(t *testing.T) {
		r := strings.NewReader("\r\n")

		want := []byte{}
		got, err := readDataBlock(r, 0)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestValidateKey(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		t.Run("min length 1", func(t *testing.T) {
			input := []byte("a")
			require.NoError(t, validateKey(input))
		})
		t.Run("max length 250", func(t *testing.T) {
			input := bytes.Repeat([]byte("a"), 250)
			require.NoError(t, validateKey(input))
		})
	})

	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
		}{
			{
				name:  "empty key",
				input: "",
			},
			{
				name:  "too long",
				input: strings.Repeat("a", maxKeyLengthInBytes+1),
			},
			{
				name:  "contains space",
				input: "test key",
			},
			{
				name:  "contains newline",
				input: "test\nkey",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				require.Error(t, validateKey([]byte(tt.input)))
			})
		}
	})
}

func TestValidateDataLen(t *testing.T) {
	tests := []struct {
		name    string
		dataLen int
		wantErr bool
	}{
		{name: "negative", dataLen: -1, wantErr: true},
		{name: "zero"},
		{name: "maximum", dataLen: maxValueSize},
		{name: "above maximum", dataLen: maxValueSize + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDataLen(tt.dataLen)

			if tt.wantErr && err == nil {
				t.Fatal("wanted err got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
