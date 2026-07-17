package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStoreCommandLine(t *testing.T) {
	validTests := []struct {
		name  string
		input string
		want  storeCommand
	}{
		{
			name:  "valid set command",
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
				key:           "test",
				flags:         127,
				expireTimeSec: 5124,
				dataLen:       4,
				omitReply:     true,
			},
		},
		{
			name:  "empty data",
			input: "test 0 0 0",
			want:  storeCommand{key: "test"},
		},
	}

	for _, tt := range validTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStoreCommandLine([]byte(tt.input))
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
