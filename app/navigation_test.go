package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/service"
)

func TestScanFailure_LeavesTheCurrentWorkAlone(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"n_{index}"}})
	require.NotEmpty(t, a.state.Previews)

	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "does-not-exist")}})

	assert.Equal(t, samples, a.state.SelectedDirectory, "a failed scan must not move the app")
	assert.NotEmpty(t, a.state.Previews, "previews belong to the directory still in view")
	assert.Contains(t, a.state.Error, "Failed to scan")
	assert.Contains(t, body, "Failed to scan")
	assert.Contains(t, body, "Rename preview", "the preview table of the folder in view is still on screen")
}

func TestScanFailure_FromNothingStaysOnTheEmptyScreen(t *testing.T) {
	a, home := newFolderApp(t)

	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "nope")}})

	assert.Empty(t, a.state.SelectedDirectory)
	assert.Contains(t, body, "Drop a folder here to begin")
}

func TestUndo_SurvivesNavigationAndSaysWhereItActs(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	docs := filepath.Join(home, "docs")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"renamed_{index}"}})
	renderPost(t, a, "/api/execute", nil)
	require.FileExists(t, filepath.Join(samples, "renamed_1.txt"))

	body := renderPost(t, a, "/api/scan", url.Values{"path": {docs}})

	assert.True(t, a.state.CanUndo, "one click in the tree must not cost the user the undo")
	assert.Contains(t, body, "Undo rename in")
	assert.Contains(t, body, "samples")

	renderPost(t, a, "/api/undo", nil)

	assert.FileExists(t, filepath.Join(samples, "a.txt"), "undo restores the folder the rename happened in")
	assert.Equal(t, docs, a.state.SelectedDirectory, "undo does not drag the user back")
	assert.False(t, a.state.CanUndo)
}

func TestUndo_InPlaceKeepsThePlainLabel(t *testing.T) {
	a, home := newFolderApp(t)
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"r_{index}"}})

	body := renderPost(t, a, "/api/execute", nil)

	assert.NotContains(t, body, "Undo rename in")
}

func TestDirectoryChange_StillAsksAfterSwitchingNamingTab(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"typed.txt"}})
	renderPost(t, a, "/api/names", url.Values{"method": {"template"}})

	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "docs")}})

	assert.Equal(t, samples, a.state.SelectedDirectory, "previews still show the typed names, so they are still at stake")
	assert.NotEmpty(t, a.state.PendingDir)
}

func TestDirectoryChange_NoQuestionOnceARuleReplacedTheTypedNames(t *testing.T) {
	a, home := newFolderApp(t)
	docs := filepath.Join(home, "docs")
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"typed.txt"}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"x_{index}"}})

	renderPost(t, a, "/api/scan", url.Values{"path": {docs}})

	assert.Equal(t, docs, a.state.SelectedDirectory)
}

func TestDiscardPrompt_GoesAwayWhenNothingTypedRemains(t *testing.T) {
	a, home := newFolderApp(t)
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"typed.txt"}})
	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "docs")}})
	require.Contains(t, body, "Discard and open")

	body = renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {""}})

	assert.NotContains(t, body, "Discard and open", "the prompt must not warn about names that are gone")
}

func TestFolderTree_PendingFolderIsMarked(t *testing.T) {
	a, home := newFolderApp(t)
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"typed.txt"}})

	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "docs")}})

	assert.Contains(t, body, `data-pending="true"`)
}

func TestFolderTree_ShowsSymlinkedFolders(t *testing.T) {
	a, home := newFolderApp(t)
	target := t.TempDir()
	require.NoError(t, os.Symlink(target, filepath.Join(home, "linked")))
	require.NoError(t, os.WriteFile(filepath.Join(home, "samples", "b.txt"), []byte("x"), 0o600))

	body := renderPost(t, a, "/api/tree/toggle", url.Values{"path": {home}})
	assert.Contains(t, body, ">linked<")

	renderPost(t, a, "/api/scan", url.Values{"path": {home}})
	for _, f := range a.state.AllFiles {
		assert.NotEqual(t, "linked", f.Name, "a link to a folder is a folder, not a file to rename")
	}
}

func TestFolderTree_KeepsTheOpenedFolderVisibleEvenWhenHidden(t *testing.T) {
	a, home := newFolderApp(t)
	cache := filepath.Join(home, ".cache")

	renderPost(t, a, "/api/scan", url.Values{"path": {cache}})
	body := renderGet(t, a, "/api/main")

	assert.False(t, a.state.ShowHiddenFiles)
	assert.Contains(t, body, ">.cache<", "the path to the folder in view must not vanish from the tree")
}

func TestFolderTree_RemembersSeveralOutsideRoots(t *testing.T) {
	a, _ := newFolderApp(t)
	first, second := t.TempDir(), t.TempDir()

	renderPost(t, a, "/api/scan", url.Values{"path": {first}})
	body := renderPost(t, a, "/api/scan", url.Values{"path": {second}})

	assert.Equal(t, []string{first, second}, a.state.ExtraRoots)
	assert.Contains(t, body, `title="`+first+`"`)
	assert.Contains(t, body, `title="`+second+`"`)
}

func TestFolderTree_ARootThatSwallowsAnotherReplacesIt(t *testing.T) {
	a, _ := newFolderApp(t)
	outer := t.TempDir()
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.Mkdir(inner, 0o750))

	renderPost(t, a, "/api/scan", url.Values{"path": {inner}})
	renderPost(t, a, "/api/scan", url.Values{"path": {outer}})

	assert.Equal(t, []string{outer}, a.state.ExtraRoots)
}

// countingFS counts directory reads so a test can see how often the tree
// touches the disk.
type countingFS struct {
	*adapterfs.OSFileSystem
	reads atomic.Int64
}

func (c *countingFS) ReadDir(path string) ([]os.DirEntry, error) {
	c.reads.Add(1)
	return c.OSFileSystem.ReadDir(path)
}

func TestFolderTree_IsNotReReadOnEveryInteraction(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"docs/reports", "samples"} {
		require.NoError(t, os.MkdirAll(filepath.Join(home, d), 0o750))
	}
	require.NoError(t, os.WriteFile(filepath.Join(home, "samples", "a.txt"), []byte("x"), 0o600))
	fs := &countingFS{OSFileSystem: &adapterfs.OSFileSystem{}}
	a := NewApp(fs, service.NewScannerService(fs), service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fs), WithHomeDir(home))
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})
	before := fs.reads.Load()

	for range 5 {
		renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"false"}})
		renderPost(t, a, "/api/pattern", url.Values{"pattern": {"a"}})
	}

	assert.Equal(t, before, fs.reads.Load(), "ticking and typing must not re-list the open folders")
}

func TestRefresh_PicksUpNewFilesAndFoldersAndKeepsTicks(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	require.NoError(t, os.WriteFile(filepath.Join(samples, "b.txt"), []byte("x"), 0o600))
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(samples, "b.txt")}, "ticked": {"false"}})

	require.NoError(t, os.WriteFile(filepath.Join(samples, "c.txt"), []byte("x"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(samples, "fresh"), 0o750))
	body := renderPost(t, a, "/api/refresh", nil)

	assert.Contains(t, body, "c.txt")
	assert.Contains(t, body, ">fresh<", "a manual refresh re-lists the folders too")
	assert.True(t, a.state.Selected[filepath.Join(samples, "c.txt")], "new files start ticked")
	assert.False(t, a.state.Selected[filepath.Join(samples, "b.txt")], "a refresh must not re-tick what the user unticked")
	assert.True(t, a.state.Selected[filepath.Join(samples, "a.txt")])
}

func TestRefresh_SilentAnswersNothingWhenNothingChanged(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})

	req := httptest.NewRequest("POST", "/api/refresh?silent=true", nil)
	rec := httptest.NewRecorder()
	a.GetHandler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code, "an unchanged folder must not swap the page under the user")

	require.NoError(t, os.WriteFile(filepath.Join(samples, "new.txt"), []byte("x"), 0o600))
	rec = httptest.NewRecorder()
	a.GetHandler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/refresh?silent=true", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "new.txt")
}

func TestRefresh_SilentIgnoresTheSortOrder(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	require.NoError(t, os.WriteFile(filepath.Join(samples, "b.txt"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(samples, "c.txt"), []byte("x"), 0o600))
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/sort", url.Values{"sort": {"name:desc"}})

	rec := httptest.NewRecorder()
	a.GetHandler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/refresh?silent=true", nil))

	assert.Equal(t, http.StatusNoContent, rec.Code, "a sorted list is not a changed list")
}

func TestToolbar_ButtonsSurviveASwapWithFocus(t *testing.T) {
	a, home := newFolderApp(t)
	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})

	for _, key := range []string{"toolbar:sidebar", "toolbar:copy", "toolbar:refresh"} {
		assert.Contains(t, body, `data-focus-key="`+key+`"`, "the swap replaces this button, so it must be findable again")
	}
}

func TestRefresh_SilentIgnoresAVanishedFolder(t *testing.T) {
	a, home := newFolderApp(t)
	gone := filepath.Join(home, "gone")
	require.NoError(t, os.Mkdir(gone, 0o750))
	renderPost(t, a, "/api/scan", url.Values{"path": {gone}})
	require.NoError(t, os.Remove(gone))

	rec := httptest.NewRecorder()
	a.GetHandler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/refresh?silent=true", nil))

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, a.state.Error, "a background check must not raise an error toast")
}

func TestTreeVisibility_CanBeHidden(t *testing.T) {
	a, _ := newFolderApp(t)
	require.Contains(t, renderGet(t, a, "/api/main"), `id="folder-tree"`)

	body := renderPost(t, a, "/api/tree/visibility", url.Values{"hide": {"true"}})
	assert.NotContains(t, body, `id="folder-tree"`)

	body = renderPost(t, a, "/api/tree/visibility", url.Values{"hide": {"false"}})
	assert.Contains(t, body, `id="folder-tree"`)
}

func TestDropFiles_SaysWhenItIgnoredTheRest(t *testing.T) {
	a, home := newFolderApp(t)

	a.DropFiles([]string{filepath.Join(home, "samples"), filepath.Join(home, "docs")}, DropTargetScan)

	assert.Equal(t, filepath.Join(home, "samples"), a.state.SelectedDirectory)
	assert.Contains(t, a.state.Error, "2 items")
	assert.True(t, strings.Contains(a.state.Error, "samples"))
}

func TestBreadcrumbBar_DeepPathKeepsHomeAndLinksTheEllipsis(t *testing.T) {
	a, home := newFolderApp(t)
	deep := filepath.Join(home, "a", "b", "c", "d", "e")
	require.NoError(t, os.MkdirAll(deep, 0o750))

	bar := breadcrumbHTML(t, renderPost(t, a, "/api/scan", url.Values{"path": {deep}}))

	assert.Contains(t, bar, ">Home<", "the way back to home stays in the bar")
	assert.Contains(t, bar, `title="`+filepath.Join(home, "a", "b")+`"`, "the ellipsis jumps to the nearest hidden ancestor")
}

func TestUndoAndShortcuts_AreAdvertisedInTheBar(t *testing.T) {
	a, home := newFolderApp(t)

	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})

	assert.Contains(t, body, `hx-post="/api/refresh"`)
	assert.Contains(t, body, "data-copy-path")
}
