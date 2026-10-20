package app

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/domain"
)

func TestAppliedNamesSurviveViewingAnotherTab(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/names", url.Values{
		"method": {"manual"}, "action": {"update"},
		"name_0": {"first"}, "name_1": {"second"}, "name_2": {"third"},
	})
	renderPost(t, a, "/api/names", url.Values{"method": {"template"}})
	renderPost(t, a, "/api/sort", url.Values{"sort": {"name:desc"}})
	assert.Equal(t, []string{"third", "second", "first"}, a.state.NewNames)
}

func TestAppliedRuleSurvivesEmptySelectionAndFilter(t *testing.T) {
	for _, empty := range []string{"selection", "filter"} {
		t.Run(empty, func(t *testing.T) {
			a, _ := newUXApp(t)
			renderPost(t, a, "/api/names/generate", url.Values{"template": {"kept_{index}"}})
			renderPost(t, a, "/api/names", url.Values{"method": {"manual"}})
			if empty == "selection" {
				renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"false"}})
				renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"true"}})
			} else {
				renderPost(t, a, "/api/pattern", url.Values{"pattern": {"no_match"}})
				renderPost(t, a, "/api/pattern", url.Values{"pattern": {""}})
			}
			require.Len(t, a.state.Previews, 3)
			assert.Equal(t, "kept_1.txt", a.state.Previews[0].NewName)
		})
	}
}

func TestManualNamesSurviveEmptySelection(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/names", url.Values{
		"method": {"manual"}, "action": {"update"},
		"name_0": {"first"}, "name_1": {"second"}, "name_2": {"third"},
	})
	renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"false"}})
	renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"true"}})
	assert.Equal(t, []string{"first", "second", "third"}, a.state.NewNames)
}

func TestDiscardClearsRememberedManualNames(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/names", url.Values{
		"method": {"manual"}, "action": {"update"}, "name_0": {"discard_me"},
	})
	renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"false"}})
	renderPost(t, a, "/api/preview", url.Values{"clear": {"true"}})
	renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"true"}})
	assert.Empty(t, a.state.NewNames)
	assert.False(t, a.hasTypedNames())
}

func TestUndoKeepsHistoryOnFailureAndCanRetry(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"new_{index}"}})
	renderPost(t, a, "/api/execute", nil)
	require.True(t, a.state.CanUndo)
	original := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(original, []byte("external"), 0o600))
	renderPost(t, a, "/api/undo", nil)
	require.True(t, a.state.CanUndo, "failed undo must remain available")
	content, err := os.ReadFile(original)
	require.NoError(t, err)
	assert.Equal(t, "external", string(content))
	require.NoError(t, os.Remove(original))
	renderPost(t, a, "/api/undo", nil)
	assert.False(t, a.state.CanUndo)
	assert.FileExists(t, original)
}

func TestUndoRestoresOverlappingNames(t *testing.T) {
	a, dir := newUXApp(t)
	before, err := os.ReadFile(filepath.Join(dir, "b.jpg"))
	require.NoError(t, err)
	renderPost(t, a, "/api/extension-policy", url.Values{"convert": {"true"}})
	renderPost(t, a, "/api/names", url.Values{
		"method": {"manual"}, "action": {"update"},
		"name_0": {"free.txt"}, "name_1": {"a.txt"}, "name_2": {"c.pdf"},
	})
	renderPost(t, a, "/api/execute", nil)
	require.True(t, a.state.CanUndo)
	renderPost(t, a, "/api/undo", nil)
	content, err := os.ReadFile(filepath.Join(dir, "b.jpg"))
	require.NoError(t, err)
	assert.Equal(t, before, content)
	assert.FileExists(t, filepath.Join(dir, "a.txt"))
}

func TestUndoRejectsHistoryOutsideItsRecordedDirectory(t *testing.T) {
	a, dir := newUXApp(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("do not move"), 0o600))
	a.state.CanUndo = true
	a.state.LastRenameDir = dir
	a.state.LastRenameHistory = []domain.RenamePreview{{
		OriginalName: "stolen.txt", NewName: "outside.txt",
		OriginalPath: filepath.Join(filepath.Dir(outside), "stolen.txt"), NewPath: outside,
	}}
	renderPost(t, a, "/api/undo", nil)
	assert.FileExists(t, outside)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(outside), "stolen.txt"))
	assert.NotEmpty(t, a.state.Error)
}

func TestExecuteRejectsPreviewOutsideSelectedDirectory(t *testing.T) {
	a, _ := newUXApp(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "outside.txt")
	require.NoError(t, os.WriteFile(source, []byte("do not move"), 0o600))
	a.state.Previews = []domain.RenamePreview{{
		OriginalName: "outside.txt", NewName: "moved.txt",
		OriginalPath: source, NewPath: filepath.Join(dir, "moved.txt"),
	}}
	renderPost(t, a, "/api/execute", nil)
	assert.FileExists(t, source)
	assert.NoFileExists(t, filepath.Join(dir, "moved.txt"))
	assert.NotEmpty(t, a.state.Error)
}

func TestNamesCannotEscapeTheCurrentDirectory(t *testing.T) {
	for _, name := range []string{"~/path/to/xxx", "../xxx", "/tmp/xxx", `..\xxx`} {
		t.Run(name, func(t *testing.T) {
			a, dir := newUXApp(t)
			renderPost(t, a, "/api/names", url.Values{
				"method": {"manual"}, "action": {"update"}, "name_0": {name},
			})
			assert.Empty(t, a.state.Previews)
			assert.NotEmpty(t, a.state.Error)
			renderPost(t, a, "/api/execute", nil)
			assert.FileExists(t, filepath.Join(dir, "a.txt"))
		})
	}
}

func TestUndoRejectsRetargetedDirectoryAlias(t *testing.T) {
	a, dir := newUXApp(t)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(dir, alias))
	renderPost(t, a, "/api/scan", url.Values{"path": {alias}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"new_{index}"}})
	renderPost(t, a, "/api/execute", nil)
	require.True(t, a.state.CanUndo)
	foreign := t.TempDir()
	for _, name := range []string{"new_1.txt", "new_2.jpg", "new_3.pdf"} {
		require.NoError(t, os.WriteFile(filepath.Join(foreign, name), []byte("foreign"), 0o600))
	}
	require.NoError(t, os.Remove(alias))
	require.NoError(t, os.Symlink(foreign, alias))
	body := renderPost(t, a, "/api/undo", nil)
	assert.True(t, a.state.CanUndo, "a failed undo must preserve its history")
	assert.Contains(t, body, "directory", "explain why undo was blocked")
	assert.NoFileExists(t, filepath.Join(foreign, "a.txt"))
	for _, name := range []string{"new_1.txt", "new_2.jpg", "new_3.pdf"} {
		content, err := os.ReadFile(filepath.Join(foreign, name))
		require.NoError(t, err)
		assert.Equal(t, "foreign", string(content))
		assert.FileExists(t, filepath.Join(dir, name), "original renamed files stay untouched")
	}
}
