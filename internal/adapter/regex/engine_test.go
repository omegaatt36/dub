package regex

import (
	"errors"
	"testing"

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
			result := e.ExpandShortcuts(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandShortcuts(%q) = %q, want %q", tt.input, result, tt.expected)
			}
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
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, domain.ErrInvalidPattern) {
					t.Errorf("expected ErrInvalidPattern, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tt.matched {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.input, matched, tt.matched)
			}
		})
	}
}
