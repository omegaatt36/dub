package app

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/service"
)

// newUXApp scans a directory whose three files differ in size, extension and
// modification time, so row/index mix-ups and ordering show up in assertions.
func newUXApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, f := range []struct {
		name string
		size int
	}{{"a.txt", 1}, {"b.jpg", 5000}, {"c.pdf", 3_000_000}} {
		path := filepath.Join(dir, f.name)
		require.NoError(t, os.WriteFile(path, make([]byte, f.size), 0o600))
		// Newest first by name order reversed: c oldest, a newest.
		mod := base.Add(time.Duration(-i) * time.Hour)
		require.NoError(t, os.Chtimes(path, mod, mod))
	}
	fs := &adapterfs.OSFileSystem{}
	a := NewApp(fs, service.NewScannerService(fs), service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fs))
	renderPost(t, a, "/api/scan", url.Values{"path": {dir}})
	return a, dir
}

func names(a *App) []string {
	var out []string
	for _, f := range a.visibleFiles() {
		out = append(out, f.Name)
	}
	return out
}

func TestSelectionChangesKeepFilterResults(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/pattern", url.Values{"pattern": {"[a-c]"}})
	require.Len(t, a.visibleFiles(), 3)

	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "a.txt")}, "ticked": {"false"}})
	assert.Len(t, a.visibleFiles(), 3, "unticking must not empty the filtered list")

	renderPost(t, a, "/api/tick-all", url.Values{"ticked": {"true"}})
	assert.Len(t, a.visibleFiles(), 3)

	renderPost(t, a, "/api/extension-policy", url.Values{"convert": {"true"}})
	assert.Len(t, a.visibleFiles(), 3)
}

func TestPreviewRowsShowTheirOwnSizeWhenSomeFilesAreUnticked(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "a.txt")}, "ticked": {"false"}})
	body := renderPost(t, a, "/api/names/generate", url.Values{"template": {"new_{index}"}})

	assert.Contains(t, body, "4.9 KB", "b.jpg keeps its own size")
	assert.Contains(t, body, "2.9 MB", "c.pdf keeps its own size")
	assert.Regexp(t, regexp.MustCompile(`>\s*1 B\s*<`), body, "unticked a.txt remains visible with its own size")
}

func TestRevealedHiddenFilesStartUnticked(t *testing.T) {
	a, _ := newHiddenFilesApp(t)
	require.Len(t, a.activeFiles(), 2)

	renderPost(t, a, "/api/hidden-files", url.Values{"show": {"true"}})
	assert.Len(t, a.visibleFiles(), 5)
	assert.Len(t, a.activeFiles(), 2, "revealing dotfiles must not make them rename targets")

	// The user can still opt in.
	renderPost(t, a, "/api/tick", url.Values{"path": {a.state.AllFiles[0].Path}, "ticked": {"true"}})
	assert.Len(t, a.activeFiles(), 3)
}

func TestHiddenFilesShownAtScanTimeAreTicked(t *testing.T) {
	a, dir := newHiddenFilesApp(t)
	renderPost(t, a, "/api/hidden-files", url.Values{"show": {"true"}})
	renderPost(t, a, "/api/scan", url.Values{"path": {dir}})
	assert.Len(t, a.activeFiles(), 5, "what the user sees at scan time is what is ticked")
}

func TestTemplateNamesFollowTheSelection(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"n_{index}"}})
	require.Len(t, a.state.Previews, 3)

	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "a.txt")}, "ticked": {"false"}})
	require.Len(t, a.state.Previews, 2, "previews are rebuilt, not discarded")
	assert.Equal(t, "n_1.jpg", a.state.Previews[0].NewName)
	assert.Equal(t, "n_2.pdf", a.state.Previews[1].NewName)
}

func TestFindReplaceNamesFollowTheFilter(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/names/findreplace", url.Values{"search": {"^(.)$"}, "replace": {"x_$1"}})
	require.Len(t, a.state.Previews, 3)

	renderPost(t, a, "/api/pattern", url.Values{"pattern": {"^b"}})
	require.Len(t, a.state.Previews, 1)
	assert.Equal(t, "x_b.jpg", a.state.Previews[0].NewName)
}

func TestManualNamesAreKeptPerFile(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/names", url.Values{
		"method": {"manual"}, "action": {"update"},
		"name_0": {"first"}, "name_1": {"second"}, "name_2": {"third"},
	})
	require.Len(t, a.state.Previews, 3)

	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "a.txt")}, "ticked": {"false"}})
	require.Len(t, a.state.Previews, 2)
	assert.Equal(t, "second.jpg", a.state.Previews[0].NewName, "b.jpg keeps the name typed for it")
	assert.Equal(t, "third.pdf", a.state.Previews[1].NewName)

	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "a.txt")}, "ticked": {"true"}})
	require.Len(t, a.state.Previews, 3)
	assert.Equal(t, "first.txt", a.state.Previews[0].NewName, "re-ticking restores what was typed")
}

func TestExtensionPolicyChangeKeepsNames(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"n_{index}.webp"}})
	assert.Equal(t, "n_1.webp.txt", a.state.Previews[0].NewName)

	renderPost(t, a, "/api/extension-policy", url.Values{"convert": {"true"}})
	require.Len(t, a.state.Previews, 3)
	assert.Equal(t, "n_1.webp", a.state.Previews[0].NewName, "the policy re-previews the same names")
}

func TestNothingAppliedMeansNothingPreviewed(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "a.txt")}, "ticked": {"false"}})
	assert.Empty(t, a.state.Previews, "ticking alone must not invent a rename")
	assert.Empty(t, a.state.NewNames)
}

func TestSortReordersAndKeepsSelection(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/tick", url.Values{"path": {filepath.Join(dir, "b.jpg")}, "ticked": {"false"}})

	renderPost(t, a, "/api/sort", url.Values{"sort": {"size:desc"}})
	assert.Equal(t, []string{"c.pdf", "b.jpg", "a.txt"}, names(a))
	assert.Len(t, a.activeFiles(), 2, "ticks survive a re-sort")

	renderPost(t, a, "/api/sort", url.Values{"sort": {"modified:asc"}})
	assert.Equal(t, []string{"c.pdf", "b.jpg", "a.txt"}, names(a), "c is oldest")

	renderPost(t, a, "/api/sort", url.Values{"sort": {"name:asc"}})
	assert.Equal(t, []string{"a.txt", "b.jpg", "c.pdf"}, names(a))
}

func TestSortOrderDrivesTemplateIndex(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/sort", url.Values{"sort": {"size:desc"}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"n_{index}"}})
	assert.Equal(t, "c.pdf", a.state.Previews[0].OriginalName)
	assert.Equal(t, "n_1.pdf", a.state.Previews[0].NewName)
}

func TestSortPersistsAcrossRescan(t *testing.T) {
	a, dir := newUXApp(t)
	renderPost(t, a, "/api/sort", url.Values{"sort": {"size:desc"}})
	renderPost(t, a, "/api/scan", url.Values{"path": {dir}})
	assert.Equal(t, []string{"c.pdf", "b.jpg", "a.txt"}, names(a))
}

func TestInvalidSortIsReportedNotApplied(t *testing.T) {
	a, _ := newUXApp(t)
	renderPost(t, a, "/api/sort", url.Values{"sort": {"color:asc"}})
	assert.NotEmpty(t, a.state.Error)
	assert.Equal(t, []string{"a.txt", "b.jpg", "c.pdf"}, names(a))
}

func TestHiddenCountAndEmptyStates(t *testing.T) {
	a, dir := newHiddenFilesApp(t)
	body := renderGet(t, a, "/api/page")
	assert.Contains(t, body, "(3)", "the toggle reports how many files it hides")

	// A folder with only dotfiles explains itself and offers the way out.
	for _, name := range []string{"photo.jpg", "file.hidden.txt"} {
		require.NoError(t, os.Remove(filepath.Join(dir, name)))
	}
	body = renderPost(t, a, "/api/scan", url.Values{"path": {dir}})
	assert.Contains(t, body, "only hidden files")
	assert.NotContains(t, body, "No files match the current filter")

	// A genuinely empty folder says so instead.
	empty := t.TempDir()
	body = renderPost(t, a, "/api/scan", url.Values{"path": {empty}})
	assert.Contains(t, body, "This folder has no files")
	assert.NotContains(t, body, "only hidden files")
}

func TestEmptyStateBeforeAnyFolderGuidesTheUser(t *testing.T) {
	fs := &adapterfs.OSFileSystem{}
	a := NewApp(fs, service.NewScannerService(fs), service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fs))
	body := renderGet(t, a, "/api/page")
	assert.Contains(t, body, "Drop a folder here")

	start := regexp.MustCompile(`<input id="show-hidden-files"[^>]*>`).FindString(body)
	require.NotEmpty(t, start)
	assert.Contains(t, start, "disabled", "nothing to reveal before a folder is chosen")
}

// TestCompoundAndUppercaseExtensionsSurviveRename drives the real scanner so
// the whole path is covered: Extension -> stem -> template/find-replace -> name.
func TestCompoundAndUppercaseExtensionsSurviveRename(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"xxx.tar.gz", "IMG_01.JPG"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600))
	}
	fs := &adapterfs.OSFileSystem{}
	a := NewApp(fs, service.NewScannerService(fs), service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fs))
	renderPost(t, a, "/api/scan", url.Values{"path": {dir}})

	renderPost(t, a, "/api/names/generate", url.Values{"template": {"yyy_{index}"}})
	got := map[string]string{}
	for _, p := range a.state.Previews {
		got[p.OriginalName] = p.NewName
	}
	assert.Equal(t, "yyy_2.tar.gz", got["xxx.tar.gz"], "the .tar must not be dropped")
	assert.Equal(t, "yyy_1.JPG", got["IMG_01.JPG"])

	renderPost(t, a, "/api/names/generate", url.Values{"template": {"{original}_x"}})
	got = map[string]string{}
	for _, p := range a.state.Previews {
		got[p.OriginalName] = p.NewName
	}
	assert.Equal(t, "xxx_x.tar.gz", got["xxx.tar.gz"])
	assert.Equal(t, "IMG_01_x.JPG", got["IMG_01.JPG"])

	renderPost(t, a, "/api/names/findreplace", url.Values{"search": {`\d+$|xxx$`}, "replace": {"N"}})
	got = map[string]string{}
	for _, p := range a.state.Previews {
		got[p.OriginalName] = p.NewName
	}
	assert.Equal(t, "N.tar.gz", got["xxx.tar.gz"])
	assert.Equal(t, "IMG_N.JPG", got["IMG_01.JPG"])

	// With the extension unlocked, dropping .tar is reported as an extension
	// change, even though both names still end in ".gz".
	renderPost(t, a, "/api/pattern", url.Values{"pattern": {"xxx"}})
	renderPost(t, a, "/api/extension-policy", url.Values{"convert": {"true"}})
	renderPost(t, a, "/api/names/generate", url.Values{"template": {"yyy.gz"}})
	require.Len(t, a.state.Previews, 1)
	assert.Equal(t, "yyy.gz", a.state.Previews[0].NewName)
	assert.Equal(t, 1, countExtensionChanges(a.state.Previews))
}
