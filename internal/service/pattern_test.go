package service

import (
	"testing"

	"github.com/omegaatt36/dub/internal/domain"
)

// MockPatternMatcher implements port.PatternMatcher for testing.
type MockPatternMatcher struct {
	ExpandShortcutsFunc func(string) string
	MatchFunc           func(string, string) (bool, error)
}

func (m *MockPatternMatcher) ExpandShortcuts(pattern string) string {
	return m.ExpandShortcutsFunc(pattern)
}

func (m *MockPatternMatcher) Match(pattern, name string) (bool, error) {
	return m.MatchFunc(pattern, name)
}

func TestPatternService_MatchFiles(t *testing.T) {
	files := []domain.FileItem{
		{Name: "file_001.txt", Extension: ".txt"},
		{Name: "file_002.txt", Extension: ".txt"},
		{Name: "photo_001.jpg", Extension: ".jpg"},
		{Name: "document.pdf", Extension: ".pdf"},
	}

	t.Run("empty pattern returns all", func(t *testing.T) {
		svc := NewPatternService(&MockPatternMatcher{})
		result, err := svc.MatchFiles(files, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != len(files) {
			t.Errorf("got %d files, want %d", len(result), len(files))
		}
	})

	t.Run("filters by pattern", func(t *testing.T) {
		pm := &MockPatternMatcher{
			ExpandShortcutsFunc: func(p string) string { return p },
			MatchFunc: func(pattern, name string) (bool, error) {
				// name is now the stem (without extension)
				return name == "file_001" || name == "file_002", nil
			},
		}

		svc := NewPatternService(pm)
		result, err := svc.MatchFiles(files, "file_")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("got %d files, want 2", len(result))
		}
	})

	t.Run("matches against stem not full filename", func(t *testing.T) {
		// Verify that the matcher receives stems, not full filenames
		var receivedNames []string
		pm := &MockPatternMatcher{
			ExpandShortcutsFunc: func(p string) string { return p },
			MatchFunc: func(pattern, name string) (bool, error) {
				receivedNames = append(receivedNames, name)
				return true, nil
			},
		}

		testFiles := []domain.FileItem{
			{Name: "hello.txt", Extension: ".txt"},
			{Name: "55688.pdf", Extension: ".pdf"},
		}

		svc := NewPatternService(pm)
		svc.MatchFiles(testFiles, "test")

		expected := []string{"hello", "55688"}
		for i, got := range receivedNames {
			if got != expected[i] {
				t.Errorf("stem[%d] = %q, want %q", i, got, expected[i])
			}
		}
	})

	t.Run("expands shortcuts before matching", func(t *testing.T) {
		var expandedPattern string
		pm := &MockPatternMatcher{
			ExpandShortcutsFunc: func(p string) string {
				expandedPattern = "expanded_" + p
				return expandedPattern
			},
			MatchFunc: func(pattern, name string) (bool, error) {
				if pattern != expandedPattern {
					t.Errorf("Match received %q, want expanded %q", pattern, expandedPattern)
				}
				return true, nil
			},
		}

		svc := NewPatternService(pm)
		_, err := svc.MatchFiles(files, "[serial]")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error on match failure", func(t *testing.T) {
		pm := &MockPatternMatcher{
			ExpandShortcutsFunc: func(p string) string { return p },
			MatchFunc: func(pattern, name string) (bool, error) {
				return false, domain.ErrInvalidPattern
			},
		}

		svc := NewPatternService(pm)
		_, err := svc.MatchFiles(files, "bad_pattern")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
