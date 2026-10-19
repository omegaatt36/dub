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

func TestSelectAllTicksOnlyAdds(t *testing.T) {
	s := NewAppState()
	s.Selected["/d/gone-from-view.txt"] = true

	s.SelectAllTicks([]domain.FileItem{{Path: "/d/a.txt"}, {Path: "/d/b.txt"}})

	assert.Equal(t, map[string]bool{"/d/gone-from-view.txt": true, "/d/a.txt": true, "/d/b.txt": true}, s.Selected,
		"ticking what is on screen must not un-tick what is not")
}

func TestPruneTicksDropsVanishedFiles(t *testing.T) {
	s := NewAppState()
	s.Selected["/d/a.txt"] = true
	s.Selected["/d/renamed-away.txt"] = true

	s.PruneTicks([]domain.FileItem{{Path: "/d/a.txt"}, {Path: "/d/hidden"}})

	assert.Equal(t, map[string]bool{"/d/a.txt": true}, s.Selected)
}

func TestSessionPreferencesSurviveDirectoryChange(t *testing.T) {
	s := NewAppState()
	s.ShowHiddenFiles = true
	s.Sort = domain.SortOrder{Field: domain.SortBySize, Desc: true}
	s.ManualNames["/d/a.txt"] = "x"

	s.ResetForDirectory()

	assert.True(t, s.ShowHiddenFiles)
	assert.Equal(t, domain.SortOrder{Field: domain.SortBySize, Desc: true}, s.Sort)
	assert.Empty(t, s.ManualNames, "names are keyed by paths of the old directory")
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
	s.ManualNames["/d/a.txt"] = "x"
	s.Previews = []domain.RenamePreview{{OriginalName: "a.txt", NewName: "b.txt"}}

	s.ResetForExecute()

	assert.Empty(t, s.ManualNames, "a rename changes the paths the names were keyed by")
	assert.Nil(t, s.NewNames)
	assert.Nil(t, s.Previews)
	assert.Equal(t, "a", s.Pattern, "filter must survive execution")
	assert.Equal(t, []domain.FileItem{{Name: "a.txt"}}, s.MatchedFiles)
}
