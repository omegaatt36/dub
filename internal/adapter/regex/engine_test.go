package regex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/domain"
)

func TestEngine_ExpandShortcuts(t *testing.T) {
	e := &Engine{}

	tests := []struct {
		input    string
		expected string
	}{
		{"[serial]", `(\d+)`},
		{"[number]", `(\d+)`},
		{"[any]", `(.*)`},
		{"[word]", `(\w+)`},
		{"[alpha]", `([a-zA-Z]+)`},
		{"prefix_[serial]_[word].txt", `prefix_(\d+)_(\w+).txt`},
		{"no shortcuts here", "no shortcuts here"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, e.ExpandShortcuts(tt.input))
		})
	}
}

func TestEngine_Match(t *testing.T) {
	e := &Engine{}

	tests := []struct {
		name    string
		pattern string
		input   string
		matched bool
		wantErr bool
	}{
		{"simple match", `file_\d+`, "file_123", true, false},
		{"no match", `file_\d+`, "photo_abc", false, false},
		{"full regex", `^IMG_\d{4}\.jpg$`, "IMG_1234.jpg", true, false},
		{"partial match", `\d+`, "abc123def", true, false},
		{"invalid regex", `[invalid`, "", false, true},
		{"empty pattern", "", "anything", true, false},
		{"dot matches", `.*\.txt`, "hello.txt", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := e.Match(tt.pattern, tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, domain.ErrInvalidPattern)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.matched, matched)
		})
	}
}
