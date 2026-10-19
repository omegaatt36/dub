package app

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/service"
)

func newHiddenFilesApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{".DS_Store", ".env", "._photo.jpg", "photo.jpg", "file.hidden.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("original"), 0o600))
	}
	fs := &adapterfs.OSFileSystem{}
	a := NewApp(fs, service.NewScannerService(fs), service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fs))
	renderPost(t, a, "/api/scan", url.Values{"path": {dir}})
	return a, dir
}

func TestHiddenFilesExcludedByDefault(t *testing.T) {
	a, dir := newHiddenFilesApp(t)
	require.Len(t, a.visibleFiles(), 2)
	require.Len(t, a.activeFiles(), 2)
	body := renderGet(t, a, "/api/page")
	assert.NotContains(t, body, ".DS_Store")
	assert.Contains(t, body, "Show hidden files")
	assert.Contains(t, body, `id="show-hidden-files"`)
	assert.Contains(t, body, `hx-post="/api/hidden-files"`)
	start := strings.Index(body, `<input id="show-hidden-files"`)
	require.GreaterOrEqual(t, start, 0)
	input := body[start : start+strings.Index(body[start:], ">")]
	assert.NotContains(t, input, "checked", "off must omit the boolean checked attribute")
	assert.Equal(t, 2, a.buildPageData(nil).TotalCount, "pattern counts exclude hidden files")
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"renamed_{index}"}})
	require.Len(t, a.state.Previews, 2)
	renderPost(t, a, "/api/execute", nil)
	for _, name := range []string{".DS_Store", ".env", "._photo.jpg"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, "original", string(content))
	}
	require.Len(t, a.visibleFiles(), 2)
	renderPost(t, a, "/api/undo", nil)
	require.Len(t, a.visibleFiles(), 2)
}

func TestHiddenFilesToggleRespectsPatternAndRebuildsPreview(t *testing.T) {
	a, _ := newHiddenFilesApp(t)
	body := renderPost(t, a, "/api/hidden-files", url.Values{"show": {"true"}})
	require.Len(t, a.visibleFiles(), 5)
	assert.Contains(t, body, ".DS_Store")

	// Revealed dotfiles are not ticked, so the rename set is unchanged.
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"renamed_{index}"}})
	require.Len(t, a.state.Previews, 2)

	// Tick one dotfile in; the template is re-applied to the new set.
	renderPost(t, a, "/api/tick", url.Values{"path": {a.state.AllFiles[0].Path}, "ticked": {"true"}})
	require.Len(t, a.state.Previews, 3)

	// Hiding drops the dotfile from the set and the previews follow.
	renderPost(t, a, "/api/hidden-files", url.Values{"show": {"false"}})
	require.Len(t, a.state.Previews, 2)
	require.Len(t, a.activeFiles(), 2)

	renderPost(t, a, "/api/pattern", url.Values{"pattern": {"photo"}})
	require.Len(t, a.visibleFiles(), 1)
	require.Len(t, a.state.Previews, 1)
	renderPost(t, a, "/api/hidden-files", url.Values{"show": {"true"}})
	require.Len(t, a.visibleFiles(), 2, "._photo.jpg matches too once revealed")
	renderPost(t, a, "/api/hidden-files", url.Values{"show": {"false"}})
	renderPost(t, a, "/api/pattern", url.Values{"pattern": {"["}})
	assert.NotEmpty(t, a.state.PatternError)
	assert.Len(t, a.visibleFiles(), 2, "invalid regex fallback must still hide dotfiles")
}

func TestHiddenFilesOnlyDirectoryCanBeRevealed(t *testing.T) {
	a, dir := newHiddenFilesApp(t)
	for _, name := range []string{"photo.jpg", "file.hidden.txt"} {
		require.NoError(t, os.Remove(filepath.Join(dir, name)))
	}
	body := renderPost(t, a, "/api/scan", url.Values{"path": {dir}})
	assert.Empty(t, a.visibleFiles())
	assert.Empty(t, a.activeFiles())
	assert.Zero(t, a.buildPageData(nil).TotalCount)
	assert.Contains(t, body, "only hidden files")
	body = renderPost(t, a, "/api/hidden-files", url.Values{"show": {"true"}})
	require.Len(t, a.visibleFiles(), 3)
	require.Empty(t, a.activeFiles(), "revealed dotfiles start unticked")
	renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"true"}})
	require.Len(t, a.activeFiles(), 3)
	start := strings.Index(body, `<input id="show-hidden-files"`)
	require.GreaterOrEqual(t, start, 0)
	input := body[start : start+strings.Index(body[start:], ">")]
	assert.Contains(t, input, "checked", "enabled must emit the boolean checked attribute")

	// The directory picker and rescans must preserve the session preference.
	a.pickDirectory = func() (string, error) { return dir, nil }
	renderPost(t, a, "/api/select-directory", nil)
	require.Len(t, a.visibleFiles(), 3)
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"revealed_{index}"}})
	require.Len(t, a.state.Previews, 3)
	renderPost(t, a, "/api/execute", nil)
	require.True(t, a.state.CanUndo)
	renderPost(t, a, "/api/undo", nil)
	require.Len(t, a.visibleFiles(), 3)
	assert.True(t, a.state.ShowHiddenFiles)
}
