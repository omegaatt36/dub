package app

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/omegaatt36/dub/internal/domain"
)

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
func (m *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

type mockFS struct {
	ReadDirFunc func(string) ([]os.DirEntry, error)
	StatFunc    func(string) (os.FileInfo, error)
	RenameFunc  func(string, string) error
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
	}
	mpm := &mockPM{}
	return NewApp(mfs, mpm)
}

func TestHandlePage(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("GET", "/api/page", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Dub") {
		t.Error("response should contain app title")
	}
}

func TestHandleScan(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	form := url.Values{"path": {"/test/dir"}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	if app.state.SelectedDirectory != "/test/dir" {
		t.Errorf("got dir %q, want %q", app.state.SelectedDirectory, "/test/dir")
	}
	if len(app.state.AllFiles) != 2 {
		t.Errorf("got %d files, want 2", len(app.state.AllFiles))
	}
}

func TestHandleScanEmptyPath(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/scan", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if app.state.Error == "" {
		t.Error("expected error for empty path")
	}
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

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleNamesGenerate(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Extension: ".txt"},
		{Name: "b.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles

	handler := app.GetHandler()

	form := url.Values{"template": {"photo_{index}"}}
	req := httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	if len(app.state.NewNames) != 2 {
		t.Fatalf("got %d names, want 2", len(app.state.NewNames))
	}
	if app.state.NewNames[0] != "photo_1" {
		t.Errorf("got name %q, want %q", app.state.NewNames[0], "photo_1")
	}
	if app.state.NewNames[1] != "photo_2" {
		t.Errorf("got name %q, want %q", app.state.NewNames[1], "photo_2")
	}
}

func TestHandlePreview(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.NewNames = []string{"renamed"}

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	if len(app.state.Previews) != 1 {
		t.Fatalf("got %d previews, want 1", len(app.state.Previews))
	}
	if app.state.Previews[0].NewName != "renamed.txt" {
		t.Errorf("got new name %q, want %q", app.state.Previews[0].NewName, "renamed.txt")
	}
}

func TestHandlePreviewClear(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.Previews = []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "b.txt"},
	}

	handler := app.GetHandler()

	form := url.Values{"clear": {"true"}}
	req := httptest.NewRequest("POST", "/api/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if len(app.state.Previews) != 0 {
		t.Error("previews should be cleared")
	}
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
	app := NewApp(mfs, mpm)

	app.state.SelectedDirectory = "/dir"
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.Previews = []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "renamed.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/renamed.txt"},
	}

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if _, ok := renamedPairs["/dir/a.txt"]; !ok {
		t.Error("expected rename to be called for a.txt")
	}
	// After execute, previews should be cleared
	if len(app.state.Previews) != 0 {
		t.Error("previews should be cleared after execute")
	}
}

func TestHandleExecuteNoPreviews(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if app.state.Error == "" {
		t.Error("expected error when no previews")
	}
}

func TestHandleNames(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Extension: ".txt"},
		{Name: "b.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles

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

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if len(app.state.NewNames) != 2 {
		t.Fatalf("got %d names, want 2", len(app.state.NewNames))
	}
	if app.state.NewNames[0] != "alpha" {
		t.Errorf("got %q, want %q", app.state.NewNames[0], "alpha")
	}
}

func TestHandleNamesUpload(t *testing.T) {
	app := newTestApp()
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles

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

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if len(app.state.NewNames) != 2 {
		t.Fatalf("got %d names, want 2", len(app.state.NewNames))
	}
	if app.state.NewNames[0] != "new_name_1" {
		t.Errorf("got %q, want %q", app.state.NewNames[0], "new_name_1")
	}
}

func TestHandleNamesUploadNoFile(t *testing.T) {
	app := newTestApp()
	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/names/upload", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if app.state.Error == "" {
		t.Error("expected error when no file uploaded")
	}
}
