package app

import (
	"bytes"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
	"github.com/omegaatt36/dub/internal/service"
)

// TestNoCORSHeaders verifies the router never advertises a wildcard CORS
// origin. Wails v2 serves the asset server on a localhost port, so a wildcard
// would let any page the user visits in a browser POST to /api/execute and
// rename files on disk without any authentication.
func TestNoCORSHeaders(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"page", http.MethodGet, "/api/page"},
		{"scan", http.MethodPost, "/api/scan"},
		{"pattern", http.MethodPost, "/api/pattern"},
		{"execute", http.MethodPost, "/api/execute"},
		{"undo", http.MethodPost, "/api/undo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			for _, header := range []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
			} {
				assert.Empty(t, rec.Header().Get(header), "unexpected %s header", header)
			}
		})
	}
}

// TestPreflightNotShortCircuited verifies OPTIONS is handled by the mux rather
// than by a global preflight handler that would answer for every path.
func TestPreflightNotShortCircuited(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest(http.MethodOptions, "/api/execute", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusOK, rec.Code, "OPTIONS should not be short-circuited with 200")
}

// TestHandleExecuteKeepsFilter verifies the active filter survives a rename so
// the user can keep working on the same subset for the next batch.
func TestHandleExecuteKeepsFilter(t *testing.T) {
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "a.txt", info: &mockFileInfo{name: "a.txt", size: 100}},
				&mockDirEntry{name: "keep.txt", info: &mockFileInfo{name: "keep.txt", size: 100}},
			}, nil
		},
		StatFunc: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		RenameFunc: func(old, new string) error { return nil },
	}
	mpm := &mockPM{
		MatchFunc: func(pattern, stem string) (bool, error) {
			return strings.HasPrefix(stem, "keep"), nil
		},
	}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(mpm),
		service.NewRenamerService(mfs),
	)
	handler := app.GetHandler()

	form := url.Values{"path": {"/dir"}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	require.Len(t, app.state.AllFiles, 2)

	form = url.Values{"pattern": {"keep"}}
	req = httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	require.Len(t, app.state.MatchedFiles, 1)

	form = url.Values{"template": {"renamed_{index}"}}
	req = httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	require.Len(t, app.state.Previews, 1)

	req = httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "keep", app.state.Pattern, "filter should survive execution")
	assert.Empty(t, app.state.NewNames, "names should be cleared after execute")
	assert.Empty(t, app.state.Previews, "previews should be cleared after execute")
	require.Len(t, app.state.MatchedFiles, 1)
	assert.Equal(t, "keep.txt", app.state.MatchedFiles[0].Name, "re-scan must re-apply the filter")
	assert.Len(t, app.visibleFiles(), 1)
}

// TestHandleUndoKeepsFilter verifies undo restores the same filtered subset.
func TestHandleUndoKeepsFilter(t *testing.T) {
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "other.txt", info: &mockFileInfo{name: "other.txt", size: 100}},
				&mockDirEntry{name: "keep.txt", info: &mockFileInfo{name: "keep.txt", size: 100}},
			}, nil
		},
		StatFunc: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		RenameFunc: func(old, new string) error { return nil },
	}
	mpm := &mockPM{
		MatchFunc: func(pattern, stem string) (bool, error) {
			return strings.HasPrefix(stem, "keep"), nil
		},
	}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(mpm),
		service.NewRenamerService(mfs),
	)
	handler := app.GetHandler()

	app.state.SelectedDirectory = "/dir"
	app.state.Pattern = "keep"
	app.state.CanUndo = true
	app.state.LastRenameHistory = []domain.RenamePreview{
		{
			OriginalName: "keep.txt",
			NewName:      "renamed.txt",
			OriginalPath: "/dir/keep.txt",
			NewPath:      "/dir/renamed.txt",
		},
	}

	req := httptest.NewRequest("POST", "/api/undo", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "keep", app.state.Pattern, "filter should survive undo")
	require.Len(t, app.state.MatchedFiles, 1)
	assert.Equal(t, "keep.txt", app.state.MatchedFiles[0].Name)
}

// TestHandlePatternInvalidRegex verifies a broken pattern reports the error and
// falls back to the full list instead of emptying the view.
func TestHandlePatternInvalidRegex(t *testing.T) {
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "a.txt", info: &mockFileInfo{name: "a.txt", size: 100}},
			}, nil
		},
		StatFunc: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(&regex.Engine{}),
		service.NewRenamerService(mfs),
	)
	handler := app.GetHandler()

	form := url.Values{"path": {"/dir"}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	form = url.Values{"pattern": {"[unclosed"}}
	req = httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, app.state.PatternError, "invalid pattern should set PatternError")
	assert.Len(t, app.state.MatchedFiles, 1, "invalid pattern should fall back to all files")
}

// TestHandlePatternClearsStalePatternError verifies a previously failing pattern
// does not leave its error behind once a valid one is entered.
func TestHandlePatternClearsStalePatternError(t *testing.T) {
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "a.txt", info: &mockFileInfo{name: "a.txt", size: 100}},
			}, nil
		},
	}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(&regex.Engine{}),
		service.NewRenamerService(mfs),
	)
	handler := app.GetHandler()

	app.state.AllFiles = []domain.FileItem{{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"}}
	app.state.PatternError = "stale error from a previous pattern"

	form := url.Values{"pattern": {"a"}}
	req := httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, app.state.PatternError)
	assert.Len(t, app.state.MatchedFiles, 1)
}

// Test mocks

type mockDirEntry struct {
	name  string
	isDir bool
	info  os.FileInfo
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode          { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) { return m.info, nil }

type mockFileInfo struct {
	name string
	size int64
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0o644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

type mockFS struct {
	ReadDirFunc  func(string) ([]os.DirEntry, error)
	StatFunc     func(string) (os.FileInfo, error)
	RenameFunc   func(string, string) error
	ReadFileFunc func(string) ([]byte, error)
}

func (m *mockFS) ReadDir(path string) ([]os.DirEntry, error) {
	if m.ReadDirFunc != nil {
		return m.ReadDirFunc(path)
	}
	return nil, nil
}

func (m *mockFS) Stat(path string) (os.FileInfo, error) {
	if m.StatFunc != nil {
		return m.StatFunc(path)
	}
	return nil, nil
}

func (m *mockFS) Rename(old, new string) error {
	if m.RenameFunc != nil {
		return m.RenameFunc(old, new)
	}
	return nil
}

func (m *mockFS) ReadFile(path string) ([]byte, error) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(path)
	}
	return nil, nil
}

type mockPM struct {
	ExpandShortcutsFunc func(string) string
	MatchFunc           func(string, string) (bool, error)
}

func (m *mockPM) ExpandShortcuts(pattern string) string {
	if m.ExpandShortcutsFunc != nil {
		return m.ExpandShortcutsFunc(pattern)
	}
	return pattern
}

func (m *mockPM) Match(pattern, name string) (bool, error) {
	if m.MatchFunc != nil {
		return m.MatchFunc(pattern, name)
	}
	return true, nil
}

func newTestApp() *App {
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "file1.txt", info: &mockFileInfo{name: "file1.txt", size: 100}},
				&mockDirEntry{name: "file2.txt", info: &mockFileInfo{name: "file2.txt", size: 200}},
			}, nil
		},
		StatFunc: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}
	mpm := &mockPM{}
	return NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(mpm),
		service.NewRenamerService(mfs),
	)
}

func TestHandlePage(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("GET", "/api/page", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Dub")
}

func TestHandleScan(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	form := url.Values{"path": {"/test/dir"}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/test/dir", app.state.SelectedDirectory)
	assert.Len(t, app.state.AllFiles, 2)
}

func TestHandleScanEmptyPath(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/scan", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, app.state.Error, "expected error for empty path")
}

func TestHandlePattern(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "file1.txt"},
		{Name: "file2.txt"},
		{Name: "photo.jpg"},
	}

	handler := app.GetHandler()

	form := url.Values{"pattern": {"file"}}
	req := httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleNamesGenerate(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Extension: ".txt"},
		{Name: "b.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)

	handler := app.GetHandler()

	form := url.Values{"template": {"photo_{index}"}}
	req := httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.NewNames, 2)
	assert.Equal(t, "photo_1", app.state.NewNames[0])
	assert.Equal(t, "photo_2", app.state.NewNames[1])
}

func TestHandlePreview(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)
	app.state.NewNames = []string{"renamed"}

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.Previews, 1)
	assert.Equal(t, "renamed.txt", app.state.Previews[0].NewName)
}

func TestHandlePreviewClear(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)
	app.state.Previews = []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "b.txt"},
	}

	handler := app.GetHandler()

	form := url.Values{"clear": {"true"}}
	req := httptest.NewRequest("POST", "/api/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, app.state.Previews)
}

func TestHandleExecute(t *testing.T) {
	renamedPairs := map[string]string{}
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "renamed.txt", info: &mockFileInfo{name: "renamed.txt", size: 100}},
			}, nil
		},
		RenameFunc: func(old, new string) error {
			renamedPairs[old] = new
			return nil
		},
	}
	mpm := &mockPM{}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(mpm),
		service.NewRenamerService(mfs),
	)

	app.state.SelectedDirectory = "/dir"
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)
	app.state.Previews = []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "renamed.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/renamed.txt"},
	}

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, renamedPairs, "/dir/a.txt")
	assert.Empty(t, app.state.Previews)
}

func TestHandleExecuteNoPreviews(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, app.state.Error, "expected error when no previews")
}

func TestHandleNames(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Extension: ".txt"},
		{Name: "b.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)

	handler := app.GetHandler()

	form := url.Values{
		"method": {"manual"},
		"action": {"update"},
		"name_0": {"alpha"},
		"name_1": {"beta"},
	}
	req := httptest.NewRequest("POST", "/api/names", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.NewNames, 2)
	assert.Equal(t, "alpha", app.state.NewNames[0])
}

func TestHandleNamesUpload(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)

	handler := app.GetHandler()

	// Build multipart form with file
	body := &strings.Builder{}
	boundary := "testboundary"
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"namesfile\"; filename=\"names.txt\"\r\n")
	body.WriteString("Content-Type: text/plain\r\n\r\n")
	body.WriteString("new_name_1\nnew_name_2\n")
	body.WriteString("\r\n--" + boundary + "--\r\n")

	req := httptest.NewRequest("POST", "/api/names/upload", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.NewNames, 2)
	assert.Equal(t, "new_name_1", app.state.NewNames[0])
}

func TestHandleNamesUploadNoFile(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/names/upload", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, app.state.Error, "expected error when no file uploaded")
}

func TestHandleExecuteLogsResult(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	renamedPairs := map[string]string{}
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "renamed.txt", info: &mockFileInfo{name: "renamed.txt", size: 100}},
			}, nil
		},
		RenameFunc: func(old, new string) error {
			renamedPairs[old] = new
			return nil
		},
	}
	mpm := &mockPM{}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(mpm),
		service.NewRenamerService(mfs),
		WithLogger(logger),
	)

	app.state.SelectedDirectory = "/dir"
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.SelectAllTicks(app.state.AllFiles)
	app.state.Previews = []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "renamed.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/renamed.txt"},
	}

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, logBuf.String(), "rename executed")
}

func TestHandleUndo(t *testing.T) {
	renamedPairs := map[string]string{}
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "a.txt", info: &mockFileInfo{name: "a.txt", size: 100}},
			}, nil
		},
		RenameFunc: func(old, new string) error {
			renamedPairs[old] = new
			return nil
		},
	}
	mpm := &mockPM{}
	app := NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(mpm),
		service.NewRenamerService(mfs),
	)

	// Simulate state after a successful execute
	app.state.SelectedDirectory = "/dir"
	app.state.CanUndo = true
	app.state.LastRenameHistory = []domain.RenamePreview{
		{
			OriginalName: "a.txt",
			NewName:      "renamed.txt",
			OriginalPath: "/dir/a.txt",
			NewPath:      "/dir/renamed.txt",
		},
	}

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/undo", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Undo should reverse: renamed.txt -> a.txt
	assert.Contains(t, renamedPairs, "/dir/renamed.txt")
	assert.Equal(t, "/dir/a.txt", renamedPairs["/dir/renamed.txt"])
	assert.False(t, app.state.CanUndo, "CanUndo should be false after undo")
}

func TestHandleUndoNothingToUndo(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/undo", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, app.state.Error)
}

// --- Selection ---

// newSelectionApp builds an app over a fixed 3-file directory.
func newSelectionApp() *App {
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "a.txt", info: &mockFileInfo{name: "a.txt", size: 100}},
				&mockDirEntry{name: "b.txt", info: &mockFileInfo{name: "b.txt", size: 200}},
				&mockDirEntry{name: "c.txt", info: &mockFileInfo{name: "c.txt", size: 300}},
			}, nil
		},
		StatFunc: func(path string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	}
	return NewApp(mfs, service.NewScannerService(mfs), service.NewPatternService(&mockPM{}), service.NewRenamerService(mfs))
}

func scanSelectionApp(t *testing.T, app *App) {
	t.Helper()
	form := url.Values{"path": {"/dir"}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)
}

// TestScanTicksEverything is the red-proof for "rename all needs no clicks".
func TestScanTicksEverything(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	assert.Len(t, app.activeFiles(), 3, "a fresh scan should tick every file")
}

// TestUntickedFileIsNeverRenamed is the core safety property: the rename must
// only touch ticked files.
func TestUntickedFileIsNeverRenamed(t *testing.T) {
	renamed := map[string]string{}
	mfs := &mockFS{
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "a.txt", info: &mockFileInfo{name: "a.txt", size: 1}},
				&mockDirEntry{name: "b.txt", info: &mockFileInfo{name: "b.txt", size: 1}},
			}, nil
		},
		StatFunc:   func(path string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		RenameFunc: func(old, nw string) error { renamed[old] = nw; return nil },
	}
	app := NewApp(mfs, service.NewScannerService(mfs), service.NewPatternService(&mockPM{}), service.NewRenamerService(mfs))
	scanSelectionApp(t, app)

	// Untick b.txt, then generate names for whatever is still active.
	form := url.Values{"path": {"/dir/b.txt"}, "ticked": {"false"}}
	req := httptest.NewRequest("POST", "/api/tick", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	genForm := url.Values{"template": {"renamed_{index}"}}
	genReq := httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), genReq)

	require.Len(t, app.state.Previews, 1, "only the ticked file should be previewed")
	assert.Equal(t, "a.txt", app.state.Previews[0].OriginalName)

	execReq := httptest.NewRequest("POST", "/api/execute", nil)
	execReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), execReq)

	assert.Contains(t, renamed, "/dir/a.txt", "the ticked file is renamed")
	assert.NotContains(t, renamed, "/dir/b.txt", "the unticked file must never be renamed")
}

// TestUntickingClearsStaleNames guards the index-coupling bug: names are bound
// to file positions, so changing the ticked set must drop them.
func TestUntickingClearsStaleNames(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	genForm := url.Values{"template": {"x_{index}"}}
	genReq := httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), genReq)
	require.Len(t, app.state.NewNames, 3)

	form := url.Values{"path": {"/dir/a.txt"}, "ticked": {"false"}}
	req := httptest.NewRequest("POST", "/api/tick", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	assert.Empty(t, app.state.NewNames, "stale names must be dropped")
	assert.Empty(t, app.state.Previews, "stale previews must be dropped")
}

// TestTickAll verifies the select-all / clear bulk controls.
func TestTickAll(t *testing.T) {
	tests := []struct {
		name       string
		ticked     string
		wantActive int
	}{
		{name: "select all", ticked: "true", wantActive: 3},
		{name: "clear all", ticked: "false", wantActive: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSelectionApp()
			scanSelectionApp(t, app)
			require.Len(t, app.activeFiles(), 3)

			form := url.Values{"ticked": {tt.ticked}}
			req := httptest.NewRequest("POST", "/api/tick-all", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

			assert.Len(t, app.activeFiles(), tt.wantActive)
		})
	}
}

// TestTickWithoutPath verifies the guard instead of silently no-oping.
func TestTickWithoutPath(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	req := httptest.NewRequest("POST", "/api/tick", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	assert.NotEmpty(t, app.state.Error)
}

// TestTicksSurvivePatternChange verifies narrowing a filter does not silently
// un-tick files the user already chose.
func TestTicksSurvivePatternChange(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	form := url.Values{"path": {"/dir/b.txt"}, "ticked": {"false"}}
	req := httptest.NewRequest("POST", "/api/tick", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	patternForm := url.Values{"pattern": {".txt"}}
	patternReq := httptest.NewRequest("POST", "/api/pattern", strings.NewReader(patternForm.Encode()))
	patternReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	assert.Len(t, app.visibleFiles(), 3)
	assert.Len(t, app.activeFiles(), 2, "the unticked file stays unticked across a filter change")
}

// TestExtensionPolicyEndpoint verifies the opt-in toggle round-trips and that
// flipping it invalidates previews computed under the other policy.
func TestExtensionPolicyEndpoint(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	assert.Equal(t, port.KeepExtension, app.state.ExtensionPolicy, "conversion must be opt-in")

	form := url.Values{"convert": {"true"}}
	req := httptest.NewRequest("POST", "/api/extension-policy", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, port.UseProposedExtension, app.state.ExtensionPolicy)
	assert.Empty(t, app.state.Previews, "changing the policy must invalidate previews")
}

// TestExtensionPolicyAppliesToGeneratedNames is the end-to-end proof that a
// template can now actually change an extension.
func TestExtensionPolicyAppliesToGeneratedNames(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	// Enable conversion before generating names.
	form := url.Values{"convert": {"true"}}
	req := httptest.NewRequest("POST", "/api/extension-policy", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), req)

	genForm := url.Values{"template": {"trip_{index}.md"}}
	genReq := httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), genReq)

	require.Len(t, app.state.Previews, 3)
	for _, p := range app.state.Previews {
		assert.Equal(t, ".md", filepath.Ext(p.NewName), "extension conversion should apply")
		assert.NotContains(t, p.NewName, ".txt", "must not stack the original extension")
	}
}

// TestKeepExtensionIsTheDefault guards the safe behaviour: without opting in,
// a template naming a different extension must not silently rewrite it.
func TestKeepExtensionIsTheDefault(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	genForm := url.Values{"template": {"trip_{index}.md"}}
	genReq := httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.GetHandler().ServeHTTP(httptest.NewRecorder(), genReq)

	require.Len(t, app.state.Previews, 3)
	assert.Equal(t, "trip_1.md.txt", app.state.Previews[0].NewName)
	assert.Equal(t, "trip_2.md.txt", app.state.Previews[1].NewName)
}

// TestResetForDirectoryRestoresSafePolicy ensures a new folder cannot inherit a
// dangerous opt-in from the previous one.
func TestResetForDirectoryRestoresSafePolicy(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	app.state.ExtensionPolicy = port.UseProposedExtension

	scanSelectionApp(t, app)
	assert.Equal(t, port.KeepExtension, app.state.ExtensionPolicy)
}
