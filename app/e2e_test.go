package app

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/service"
)

// TestE2E_FullRenameFlow tests the complete flow:
// scan directory → upload names file → preview → execute rename
// using real filesystem (no mocks).
func TestE2E_FullRenameFlow(t *testing.T) {
	// Setup: create test files in a temp directory
	dir := t.TempDir()
	for i := 1; i <= 20; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file_%d.pdf", i))
		require.NoError(t, os.WriteFile(path, []byte{}, 0o644))
	}

	// Wire real adapters (no mocks)
	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(
		realFS,
		service.NewScannerService(realFS),
		service.NewPatternService(realPM),
		service.NewRenamerService(realFS),
	)
	handler := app.GetHandler()

	// Step 1: Scan directory
	t.Log("Step 1: Scan directory")
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.AllFiles, 20)
	t.Logf("  Scanned %d files", len(app.state.AllFiles))

	// Verify natural sort: file_1, file_2, ..., file_10, ..., file_20
	assert.Equal(t, "file_1.pdf", app.state.AllFiles[0].Name)
	assert.Equal(t, "file_10.pdf", app.state.AllFiles[9].Name)

	// Step 2: Filter by pattern
	t.Log("Step 2: Filter by pattern (file_)")
	form = url.Values{"pattern": {`file_\d+`}}
	req = httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.MatchedFiles, 20)
	t.Logf("  Matched %d files", len(app.state.MatchedFiles))

	// Step 3: Upload names file
	t.Log("Step 3: Upload names file (a-t)")
	namesContent := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\nq\nr\ns\nt\n"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("namesfile", "names.txt")
	require.NoError(t, err)
	_, _ = part.Write([]byte(namesContent))
	_ = writer.Close()

	req = httptest.NewRequest("POST", "/api/names/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.NewNames, 20)
	assert.Equal(t, "a", app.state.NewNames[0])
	t.Logf("  Loaded %d names", len(app.state.NewNames))

	// Step 4: Preview
	t.Log("Step 4: Preview rename")
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.Previews, 20)

	// Check preview details
	p := app.state.Previews[0]
	assert.Equal(t, "file_1.pdf", p.OriginalName)
	assert.Equal(t, "a.pdf", p.NewName)

	// Check no conflicts
	for i, p := range app.state.Previews {
		assert.False(t, p.Conflict, "preview[%d]: unexpected conflict for %q -> %q", i, p.OriginalName, p.NewName)
	}
	t.Logf("  Generated %d previews, 0 conflicts", len(app.state.Previews))

	// Step 5: Execute rename
	t.Log("Step 5: Execute rename")
	req = httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Verify files on disk were actually renamed
	expectedFiles := []string{
		"a.pdf", "b.pdf", "c.pdf", "d.pdf", "e.pdf",
		"f.pdf", "g.pdf", "h.pdf", "i.pdf", "j.pdf",
		"k.pdf", "l.pdf", "m.pdf", "n.pdf", "o.pdf",
		"p.pdf", "q.pdf", "r.pdf", "s.pdf", "t.pdf",
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	actualNames := make(map[string]bool)
	for _, e := range entries {
		actualNames[e.Name()] = true
	}

	for _, expected := range expectedFiles {
		assert.True(t, actualNames[expected], "expected file %q not found after rename", expected)
	}

	// Verify old files are gone
	for i := 1; i <= 20; i++ {
		old := fmt.Sprintf("file_%d.pdf", i)
		assert.False(t, actualNames[old], "old file %q still exists after rename", old)
	}

	t.Logf("  Verified %d files renamed successfully on disk", len(expectedFiles))

	// Verify state was reset
	assert.Empty(t, app.state.Previews, "previews should be cleared after execute")
	assert.Empty(t, app.state.NewNames, "names should be cleared after execute")

	// Verify re-scan happened
	assert.Len(t, app.state.AllFiles, 20)
	assert.Equal(t, "a.pdf", app.state.AllFiles[0].Name)
	t.Log("  State reset and re-scan verified")
}

// TestE2E_TemplateRenameFlow tests: scan → template generate → preview → execute
func TestE2E_TemplateRenameFlow(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		path := filepath.Join(dir, fmt.Sprintf("photo_%d.jpg", i))
		_ = os.WriteFile(path, []byte{}, 0o644)
	}

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(
		realFS,
		service.NewScannerService(realFS),
		service.NewPatternService(realPM),
		service.NewRenamerService(realFS),
	)
	handler := app.GetHandler()

	// Scan
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Len(t, app.state.AllFiles, 5)

	// Generate with template
	form = url.Values{"template": {"vacation_{index}"}}
	req = httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Len(t, app.state.NewNames, 5)
	assert.Equal(t, "vacation_1", app.state.NewNames[0])

	// Preview
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "vacation_1.jpg", app.state.Previews[0].NewName)

	// Execute
	req = httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify on disk
	entries, _ := os.ReadDir(dir)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}

	for i := 1; i <= 5; i++ {
		expected := fmt.Sprintf("vacation_%d.jpg", i)
		assert.True(t, slices.Contains(names, expected), "expected %q not found, got %v", expected, names)
	}
}

// TestE2E_AlphaPatternExcludesNumeric verifies [alpha] matches letter-only stems
// and excludes purely numeric filenames like 55688.pdf.
func TestE2E_AlphaPatternExcludesNumeric(t *testing.T) {
	dir := t.TempDir()
	// Create mix of alpha and numeric filenames
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf", "55688.pdf", "123.pdf"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644)
	}

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(
		realFS,
		service.NewScannerService(realFS),
		service.NewPatternService(realPM),
		service.NewRenamerService(realFS),
	)
	handler := app.GetHandler()

	// Scan
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Len(t, app.state.AllFiles, 5)

	// Filter with [alpha] — should match only a, b, c (not 55688, 123)
	form = url.Values{"pattern": {"[alpha]"}}
	req = httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	matched := app.state.MatchedFiles
	require.Len(t, matched, 3, "[alpha] should match only a, b, c")

	for _, f := range matched {
		assert.NotEqual(t, "55688.pdf", f.Name, "[alpha] should NOT match numeric stems")
		assert.NotEqual(t, "123.pdf", f.Name, "[alpha] should NOT match numeric stems")
	}
	t.Logf("  [alpha] correctly matched %d files, excluded numeric stems", len(matched))
}

// TestE2E_ConflictPrevention tests that conflicting names are NOT renamed.
func TestE2E_ConflictPrevention(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc"), 0o644)

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(
		realFS,
		service.NewScannerService(realFS),
		service.NewPatternService(realPM),
		service.NewRenamerService(realFS),
	)
	handler := app.GetHandler()

	// Scan
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Set names: two files get the same name "same"
	app.state.NewNames = []string{"same", "same", "unique"}

	// Preview
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Both "same" entries should be conflicts
	conflicts := 0
	for _, p := range app.state.Previews {
		if p.Conflict {
			conflicts++
		}
	}
	assert.Equal(t, 2, conflicts)

	// Execute — should only rename the non-conflict file
	req = httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// a.txt and b.txt should still exist (conflict, not renamed)
	entries, _ := os.ReadDir(dir)
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}

	assert.True(t, names["a.txt"], "a.txt should still exist (conflict)")
	assert.True(t, names["b.txt"], "b.txt should still exist (conflict)")
	assert.True(t, names["unique.txt"], "unique.txt should exist (renamed from c.txt)")
	assert.False(t, names["c.txt"], "c.txt should be gone (renamed to unique.txt)")
}

// TestE2E_UndoRename tests: scan → generate names → preview → execute → undo
func TestE2E_UndoRename(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file_%d.txt", i))
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("content_%d", i)), 0o644))
	}

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(
		realFS,
		service.NewScannerService(realFS),
		service.NewPatternService(realPM),
		service.NewRenamerService(realFS),
	)
	handler := app.GetHandler()

	// Step 1: Scan
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Len(t, app.state.AllFiles, 3)

	// Step 2: Generate names
	form = url.Values{"template": {"renamed_{index}"}}
	req = httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Len(t, app.state.NewNames, 3)

	// Step 3: Preview
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Len(t, app.state.Previews, 3)

	// Step 4: Execute
	req = httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, app.state.CanUndo, "CanUndo should be true after execute")

	// Verify files were renamed
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	renamedNames := make([]string, len(entries))
	for i, e := range entries {
		renamedNames[i] = e.Name()
	}
	assert.Contains(t, renamedNames, "renamed_1.txt")
	assert.Contains(t, renamedNames, "renamed_2.txt")
	assert.Contains(t, renamedNames, "renamed_3.txt")

	// Step 5: Undo
	req = httptest.NewRequest("POST", "/api/undo", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, app.state.CanUndo, "CanUndo should be false after undo")

	// Verify files were restored to original names
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	restoredNames := make([]string, len(entries))
	for i, e := range entries {
		restoredNames[i] = e.Name()
	}
	assert.Contains(t, restoredNames, "file_1.txt")
	assert.Contains(t, restoredNames, "file_2.txt")
	assert.Contains(t, restoredNames, "file_3.txt")
	assert.NotContains(t, restoredNames, "renamed_1.txt")

	// Verify file contents preserved
	content, err := os.ReadFile(filepath.Join(dir, "file_1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content_1", string(content))
}
