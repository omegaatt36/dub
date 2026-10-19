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

// newFolderApp builds an app on the real filesystem with home pointing at a
// temp tree: home/{docs/{reports,archive},samples/{raw},.cache}, plus a file
// in samples so a scan has something to list.
func newFolderApp(t *testing.T) (*App, string) {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{"docs/reports", "docs/archive", "samples/raw", ".cache"} {
		require.NoError(t, os.MkdirAll(filepath.Join(home, d), 0o750))
	}
	require.NoError(t, os.WriteFile(filepath.Join(home, "samples", "a.txt"), []byte("x"), 0o600))

	fs := &adapterfs.OSFileSystem{}
	a := NewApp(fs, service.NewScannerService(fs), service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fs), WithHomeDir(home))
	return a, home
}

func TestFolderTree_HomeIsRootBeforeAnythingIsOpened(t *testing.T) {
	a, _ := newFolderApp(t)

	body := renderGet(t, a, "/api/page")

	assert.Contains(t, body, `id="folder-tree"`)
	assert.Contains(t, body, ">Home<")
	assert.NotContains(t, body, ">docs<", "home starts collapsed")
}

func TestFolderTree_ClickingAFolderScansItWithoutTheDialog(t *testing.T) {
	a, home := newFolderApp(t)

	renderPost(t, a, "/api/tree/toggle", url.Values{"path": {home}})
	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})

	assert.Equal(t, filepath.Join(home, "samples"), a.state.SelectedDirectory)
	assert.Contains(t, body, "a.txt")
	assert.Contains(t, body, ">raw<", "the opened folder shows its subfolders so the user can keep going down")
}

func TestFolderTree_NavigationRevealsTheChainAndKeepsItCollapsible(t *testing.T) {
	a, home := newFolderApp(t)
	target := filepath.Join(home, "docs", "reports")

	renderPost(t, a, "/api/scan", url.Values{"path": {target}})

	for _, dir := range []string{home, filepath.Join(home, "docs"), target} {
		assert.True(t, a.state.Expanded[dir], "%s should be open", dir)
	}

	body := renderPost(t, a, "/api/tree/toggle", url.Values{"path": {filepath.Join(home, "docs")}})
	assert.NotContains(t, body, ">reports<", "a revealed ancestor can still be collapsed")
	assert.Contains(t, body, ">docs<")
}

func TestFolderTree_ToggleSwapsOnlyTheTree(t *testing.T) {
	a, home := newFolderApp(t)

	body := renderPost(t, a, "/api/tree/toggle", url.Values{"path": {home}})

	assert.Contains(t, body, `id="folder-tree"`)
	assert.NotContains(t, body, `id="file-list"`, "browsing must not re-render the file list")
	assert.Contains(t, body, ">docs<")
	assert.Contains(t, body, ">samples<")
}

func TestFolderTree_HiddenFoldersFollowTheHiddenFilesSetting(t *testing.T) {
	a, home := newFolderApp(t)
	// Opening a directory under home leaves home itself open in the tree.
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "samples")}})

	assert.NotContains(t, renderGet(t, a, "/api/main"), ">.cache<")

	renderPost(t, a, "/api/hidden-files", url.Values{"show": {"true"}})
	assert.Contains(t, renderGet(t, a, "/api/main"), ">.cache<")
}

func TestFolderTree_OutsideHomeBecomesAnExtraRootThatSticks(t *testing.T) {
	a, _ := newFolderApp(t)
	outside := t.TempDir()
	child := filepath.Join(outside, "inner")
	require.NoError(t, os.Mkdir(child, 0o750))

	renderPost(t, a, "/api/scan", url.Values{"path": {outside}})
	require.Equal(t, []string{outside}, a.state.ExtraRoots)

	body := renderPost(t, a, "/api/scan", url.Values{"path": {child}})

	assert.Equal(t, []string{outside}, a.state.ExtraRoots, "navigating inside the root must not move it, or the way back up is lost")
	assert.Contains(t, body, ">inner<")
	assert.Contains(t, body, ">"+filepath.Base(outside)+"<")
}

func TestFolderTree_UnreadableFolderIsReportedInPlace(t *testing.T) {
	a, home := newFolderApp(t)
	locked := filepath.Join(home, "docs")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("running with privileges that ignore directory permissions")
	}

	renderPost(t, a, "/api/tree/toggle", url.Values{"path": {home}})
	body := renderPost(t, a, "/api/tree/toggle", url.Values{"path": {locked}})

	assert.Contains(t, body, "Cannot open this folder")
	assert.Empty(t, a.state.Error, "a locked folder in the tree is not an app-level error")
}

func TestBreadcrumbs(t *testing.T) {
	t.Run("under home starts at Home", func(t *testing.T) {
		got := breadcrumbs("/Users/me/dev/dub", "/Users/me")

		require.Len(t, got, 3)
		assert.Equal(t, "Home", got[0].Name)
		assert.Equal(t, "/Users/me", got[0].Path)
		assert.Equal(t, "dev", got[1].Name)
		assert.Equal(t, "/Users/me/dev", got[1].Path)
		assert.Equal(t, "/Users/me/dev/dub", got[2].Path)
	})

	t.Run("home itself is a single crumb", func(t *testing.T) {
		got := breadcrumbs("/Users/me", "/Users/me")
		require.Len(t, got, 1)
		assert.Equal(t, "Home", got[0].Name)
	})

	t.Run("outside home starts at root", func(t *testing.T) {
		got := breadcrumbs("/Volumes/Disk/photos", "/Users/me")

		require.Len(t, got, 4)
		assert.Equal(t, "/", got[0].Name)
		assert.Equal(t, "Volumes", got[1].Name)
		assert.Equal(t, "/Volumes/Disk/photos", got[3].Path)
	})

	t.Run("a sibling that shares the home prefix is not under home", func(t *testing.T) {
		got := breadcrumbs("/Users/me2/x", "/Users/me")
		assert.Equal(t, "/", got[0].Name)
	})

	t.Run("no directory no crumbs", func(t *testing.T) {
		assert.Empty(t, breadcrumbs("", "/Users/me"))
	})
}

func TestBreadcrumbBar_FollowsDirectoryChanges(t *testing.T) {
	a, home := newFolderApp(t)

	body := renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "docs", "reports")}})
	bar := breadcrumbHTML(t, body)

	assert.Contains(t, bar, `aria-current="page">reports<`)
	assert.Contains(t, bar, `title="`+filepath.Join(home, "docs")+`"`, "each ancestor links back up")
	assert.NotContains(t, body, "No folder selected")
}

func TestBreadcrumbBar_ShowsTheTailOfADeepPath(t *testing.T) {
	a, home := newFolderApp(t)
	deep := filepath.Join(home, "a", "b", "c", "d", "e")
	require.NoError(t, os.MkdirAll(deep, 0o750))

	bar := breadcrumbHTML(t, renderPost(t, a, "/api/scan", url.Values{"path": {deep}}))

	assert.Contains(t, bar, ">e<")
	assert.Contains(t, bar, "…")
	assert.NotContains(t, bar, `title="`+filepath.Join(home, "a")+`"`, "the head of a deep path is dropped from the bar")
}

// breadcrumbHTML cuts the path bar out of a page. The folder tree carries the
// same paths as titles, so asserting on the whole page would match both.
func breadcrumbHTML(t *testing.T, body string) string {
	t.Helper()
	_, rest, ok := strings.Cut(body, `aria-label="Current folder"`)
	require.True(t, ok, "page has no path bar")
	bar, _, ok := strings.Cut(rest, "</nav>")
	require.True(t, ok)
	return bar
}

func TestDirectoryChange_AsksBeforeDiscardingTypedNames(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	docs := filepath.Join(home, "docs")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"renamed.txt"}})

	body := renderPost(t, a, "/api/scan", url.Values{"path": {docs}})

	assert.Equal(t, samples, a.state.SelectedDirectory, "the directory must not change until the user agrees")
	assert.Equal(t, docs, a.state.PendingDir)
	assert.Contains(t, body, "Discard and open")
	assert.Contains(t, body, "Keep editing")
	assert.Equal(t, []string{"renamed.txt"}, a.state.NewNames, "typed names survive the question")
}

func TestDirectoryChange_DiscardOpensAndDropsTheNames(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	docs := filepath.Join(home, "docs")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"renamed.txt"}})
	renderPost(t, a, "/api/scan", url.Values{"path": {docs}})

	body := renderPost(t, a, "/api/scan", url.Values{"path": {docs}, "discard": {"true"}})

	assert.Equal(t, docs, a.state.SelectedDirectory)
	assert.Empty(t, a.state.PendingDir)
	assert.Empty(t, a.state.NewNames)
	assert.NotContains(t, body, "Discard and open")
}

func TestDirectoryChange_KeepEditingStaysPut(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"renamed.txt"}})
	renderPost(t, a, "/api/scan", url.Values{"path": {filepath.Join(home, "docs")}})

	body := renderPost(t, a, "/api/scan/cancel", nil)

	assert.Equal(t, samples, a.state.SelectedDirectory)
	assert.Empty(t, a.state.PendingDir)
	assert.NotContains(t, body, "Discard and open")
	assert.Equal(t, []string{"renamed.txt"}, a.state.NewNames)
}

func TestDirectoryChange_NoQuestionWhenNothingTypedIsAtStake(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	docs := filepath.Join(home, "docs")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})

	t.Run("untouched manual editor", func(t *testing.T) {
		renderPost(t, a, "/api/scan", url.Values{"path": {docs}})
		assert.Equal(t, docs, a.state.SelectedDirectory)
	})

	t.Run("a template is a rule, not typed work", func(t *testing.T) {
		renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
		renderPost(t, a, "/api/names/generate", url.Values{"template": {"x_{index}"}})

		renderPost(t, a, "/api/scan", url.Values{"path": {docs}})

		assert.Equal(t, docs, a.state.SelectedDirectory)
		assert.Empty(t, a.state.PendingDir)
	})
}

func TestDirectoryChange_DropIsGuardedToo(t *testing.T) {
	a, home := newFolderApp(t)
	samples := filepath.Join(home, "samples")
	renderPost(t, a, "/api/scan", url.Values{"path": {samples}})
	renderPost(t, a, "/api/names", url.Values{"method": {"manual"}, "action": {"update"}, "name_0": {"renamed.txt"}})

	a.DropFiles([]string{filepath.Join(home, "docs", "reports")}, DropTargetScan)

	assert.Equal(t, samples, a.state.SelectedDirectory)
	assert.Equal(t, filepath.Join(home, "docs", "reports"), a.state.PendingDir)
}
