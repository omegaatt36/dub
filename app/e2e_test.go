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
	"strings"
	"testing"

	adapterfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
)

// TestE2E_FullRenameFlow tests the complete flow:
// scan directory → upload names file → preview → execute rename
// using real filesystem (no mocks).
func TestE2E_FullRenameFlow(t *testing.T) {
	// Setup: create test files in a temp directory
	dir := t.TempDir()
	for i := 1; i <= 20; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file_%d.pdf", i))
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// Wire real adapters (no mocks)
	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(realFS, realPM)
	handler := app.GetHandler()

	// Step 1: Scan directory
	t.Log("Step 1: Scan directory")
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("scan: got status %d", rec.Code)
	}
	if len(app.state.AllFiles) != 20 {
		t.Fatalf("scan: got %d files, want 20", len(app.state.AllFiles))
	}
	t.Logf("  Scanned %d files", len(app.state.AllFiles))

	// Verify natural sort: file_1, file_2, ..., file_10, ..., file_20
	if app.state.AllFiles[0].Name != "file_1.pdf" {
		t.Errorf("sort: first file = %q, want file_1.pdf", app.state.AllFiles[0].Name)
	}
	if app.state.AllFiles[9].Name != "file_10.pdf" {
		t.Errorf("sort: 10th file = %q, want file_10.pdf", app.state.AllFiles[9].Name)
	}

	// Step 2: Filter by pattern
	t.Log("Step 2: Filter by pattern (file_)")
	form = url.Values{"pattern": {`file_\d+`}}
	req = httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("pattern: got status %d", rec.Code)
	}
	if len(app.state.MatchedFiles) != 20 {
		t.Fatalf("pattern: got %d matched, want 20", len(app.state.MatchedFiles))
	}
	t.Logf("  Matched %d files", len(app.state.MatchedFiles))

	// Step 3: Upload names file
	t.Log("Step 3: Upload names file (a-t)")
	namesContent := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\nq\nr\ns\nt\n"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("namesfile", "names.txt")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	part.Write([]byte(namesContent))
	writer.Close()

	req = httptest.NewRequest("POST", "/api/names/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("upload: got status %d", rec.Code)
	}
	if len(app.state.NewNames) != 20 {
		t.Fatalf("upload: got %d names, want 20", len(app.state.NewNames))
	}
	if app.state.NewNames[0] != "a" {
		t.Errorf("upload: first name = %q, want %q", app.state.NewNames[0], "a")
	}
	t.Logf("  Loaded %d names", len(app.state.NewNames))

	// Step 4: Preview
	t.Log("Step 4: Preview rename")
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("preview: got status %d", rec.Code)
	}
	if len(app.state.Previews) != 20 {
		t.Fatalf("preview: got %d previews, want 20", len(app.state.Previews))
	}

	// Check preview details
	p := app.state.Previews[0]
	if p.OriginalName != "file_1.pdf" {
		t.Errorf("preview[0]: original = %q, want file_1.pdf", p.OriginalName)
	}
	if p.NewName != "a.pdf" {
		t.Errorf("preview[0]: new = %q, want a.pdf", p.NewName)
	}

	// Check no conflicts
	for i, p := range app.state.Previews {
		if p.Conflict {
			t.Errorf("preview[%d]: unexpected conflict for %q -> %q", i, p.OriginalName, p.NewName)
		}
	}
	t.Logf("  Generated %d previews, 0 conflicts", len(app.state.Previews))

	// Step 5: Execute rename
	t.Log("Step 5: Execute rename")
	req = httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("execute: got status %d", rec.Code)
	}

	// Verify files on disk were actually renamed
	expectedFiles := []string{
		"a.pdf", "b.pdf", "c.pdf", "d.pdf", "e.pdf",
		"f.pdf", "g.pdf", "h.pdf", "i.pdf", "j.pdf",
		"k.pdf", "l.pdf", "m.pdf", "n.pdf", "o.pdf",
		"p.pdf", "q.pdf", "r.pdf", "s.pdf", "t.pdf",
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir after rename: %v", err)
	}

	actualNames := make(map[string]bool)
	for _, e := range entries {
		actualNames[e.Name()] = true
	}

	for _, expected := range expectedFiles {
		if !actualNames[expected] {
			t.Errorf("expected file %q not found after rename", expected)
		}
	}

	// Verify old files are gone
	for i := 1; i <= 20; i++ {
		old := fmt.Sprintf("file_%d.pdf", i)
		if actualNames[old] {
			t.Errorf("old file %q still exists after rename", old)
		}
	}

	t.Logf("  Verified %d files renamed successfully on disk", len(expectedFiles))

	// Verify state was reset
	if len(app.state.Previews) != 0 {
		t.Error("previews should be cleared after execute")
	}
	if len(app.state.NewNames) != 0 {
		t.Error("names should be cleared after execute")
	}

	// Verify re-scan happened
	if len(app.state.AllFiles) != 20 {
		t.Errorf("re-scan: got %d files, want 20", len(app.state.AllFiles))
	}
	if app.state.AllFiles[0].Name != "a.pdf" {
		t.Errorf("re-scan: first file = %q, want a.pdf", app.state.AllFiles[0].Name)
	}
	t.Log("  State reset and re-scan verified")
}

// TestE2E_TemplateRenameFlow tests: scan → template generate → preview → execute
func TestE2E_TemplateRenameFlow(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		path := filepath.Join(dir, fmt.Sprintf("photo_%d.jpg", i))
		os.WriteFile(path, []byte{}, 0644)
	}

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(realFS, realPM)
	handler := app.GetHandler()

	// Scan
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(app.state.AllFiles) != 5 {
		t.Fatalf("scan: got %d files, want 5", len(app.state.AllFiles))
	}

	// Generate with template
	form = url.Values{"template": {"vacation_{index}"}}
	req = httptest.NewRequest("POST", "/api/names/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(app.state.NewNames) != 5 {
		t.Fatalf("generate: got %d names, want 5", len(app.state.NewNames))
	}
	if app.state.NewNames[0] != "vacation_1" {
		t.Errorf("generate: first name = %q, want vacation_1", app.state.NewNames[0])
	}

	// Preview
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if app.state.Previews[0].NewName != "vacation_1.jpg" {
		t.Errorf("preview: new name = %q, want vacation_1.jpg", app.state.Previews[0].NewName)
	}

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
		found := false
		for _, n := range names {
			if n == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q not found, got %v", expected, names)
		}
	}
}

// TestE2E_AlphaPatternExcludesNumeric verifies [alpha] matches letter-only stems
// and excludes purely numeric filenames like 55688.pdf.
func TestE2E_AlphaPatternExcludesNumeric(t *testing.T) {
	dir := t.TempDir()
	// Create mix of alpha and numeric filenames
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf", "55688.pdf", "123.pdf"} {
		os.WriteFile(filepath.Join(dir, name), []byte{}, 0644)
	}

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(realFS, realPM)
	handler := app.GetHandler()

	// Scan
	form := url.Values{"path": {dir}}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(app.state.AllFiles) != 5 {
		t.Fatalf("scan: got %d files, want 5", len(app.state.AllFiles))
	}

	// Filter with [alpha] — should match only a, b, c (not 55688, 123)
	form = url.Values{"pattern": {"[alpha]"}}
	req = httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	matched := app.state.MatchedFiles
	if len(matched) != 3 {
		names := make([]string, len(matched))
		for i, f := range matched {
			names[i] = f.Name
		}
		t.Fatalf("[alpha] matched %d files %v, want 3 (a,b,c only)", len(matched), names)
	}

	for _, f := range matched {
		if f.Name == "55688.pdf" || f.Name == "123.pdf" {
			t.Errorf("[alpha] should NOT match %q", f.Name)
		}
	}
	t.Logf("  [alpha] correctly matched %d files, excluded numeric stems", len(matched))
}

// TestE2E_ConflictPrevention tests that conflicting names are NOT renamed.
func TestE2E_ConflictPrevention(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc"), 0644)

	realFS := &adapterfs.OSFileSystem{}
	realPM := &regex.Engine{}
	app := NewApp(realFS, realPM)
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
	if conflicts != 2 {
		t.Errorf("expected 2 conflicts, got %d", conflicts)
	}

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

	if !names["a.txt"] {
		t.Error("a.txt should still exist (conflict)")
	}
	if !names["b.txt"] {
		t.Error("b.txt should still exist (conflict)")
	}
	if !names["unique.txt"] {
		t.Error("unique.txt should exist (renamed from c.txt)")
	}
	if names["c.txt"] {
		t.Error("c.txt should be gone (renamed to unique.txt)")
	}
}
