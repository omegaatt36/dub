package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/omegaatt36/dub/internal/domain"
)

func TestResetForDirectoryClearsEverything(t *testing.T) {
	s := NewAppState()
	s.AllFiles = []domain.FileItem{{Name: "a.txt"}}
	s.MatchedFiles = []domain.FileItem{{Name: "a.txt"}}
	s.Pattern = "a"
	s.PatternError = "boom"
	s.NewNames = []string{"x"}
	s.Previews = []domain.RenamePreview{{OriginalName: "a.txt", NewName: "b.txt"}}
	s.Error = "boom"
	s.CanUndo = true
	s.LastRenameHistory = []domain.RenamePreview{{OriginalName: "a.txt", NewName: "b.txt"}}

	s.ResetForDirectory()

	assert.Nil(t, s.AllFiles)
	assert.Nil(t, s.MatchedFiles)
	assert.Empty(t, s.Pattern)
	assert.Empty(t, s.PatternError, "PatternError must not be left stale")
	assert.Nil(t, s.NewNames)
	assert.Nil(t, s.Previews)
	assert.Empty(t, s.Error)
	assert.False(t, s.CanUndo)
	assert.Nil(t, s.LastRenameHistory)
}

func TestResetForPatternClearsMatchDependentState(t *testing.T) {
	s := NewAppState()
	s.AllFiles = []domain.FileItem{{Name: "a.txt"}}
	s.MatchedFiles = []domain.FileItem{{Name: "a.txt"}}
	s.PatternError = "boom"
	s.NewNames = []string{"x"}
	s.Previews = []domain.RenamePreview{{OriginalName: "a.txt", NewName: "b.txt"}}

	s.ResetForPattern()

	assert.Nil(t, s.MatchedFiles)
	assert.Nil(t, s.NewNames)
	assert.Nil(t, s.Previews)
	assert.Empty(t, s.PatternError, "PatternError must not be left stale")
	assert.Len(t, s.AllFiles, 1, "the scanned list must survive a pattern change")
}

func TestClearPreviewsKeepsNamesAndFilter(t *testing.T) {
	s := NewAppState()
	s.Pattern = "a"
	s.NewNames = []string{"x"}
	s.Previews = []domain.RenamePreview{{OriginalName: "a.txt", NewName: "b.txt"}}

	s.ClearPreviews()

	assert.Nil(t, s.Previews)
	assert.Equal(t, "a", s.Pattern)
	assert.Equal(t, []string{"x"}, s.NewNames)
}

func TestResetForExecuteKeepsFilter(t *testing.T) {
	s := NewAppState()
	s.AllFiles = []domain.FileItem{{Name: "a.txt"}, {Name: "b.txt"}}
	s.MatchedFiles = []domain.FileItem{{Name: "a.txt"}}
	s.Pattern = "a"
	s.NewNames = []string{"x"}
	s.Previews = []domain.RenamePreview{{OriginalName: "a.txt", NewName: "b.txt"}}

	s.ResetForExecute()

	assert.Nil(t, s.NewNames)
	assert.Nil(t, s.Previews)
	assert.Equal(t, "a", s.Pattern, "filter must survive execution")
	assert.Equal(t, []domain.FileItem{{Name: "a.txt"}}, s.MatchedFiles)
}
