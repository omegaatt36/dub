package app

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestNamesFromLinesLimits(t *testing.T) {
	for _, test := range []struct {
		name, content, message string
	}{
		{"long line", "valid\n" + strings.Repeat("x", maxNameLineBytes+1), "line"},
		{"scanner long line", strings.Repeat("x", maxNameLineBytes*2), "line"},
		{"too many names", strings.Repeat("a\n", maxNamesCount+1), "100000 names"},
		{"too many bytes", strings.Repeat("\n", maxNamesBytes+1), "4194304 bytes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			names, err := namesFromLines(strings.NewReader(test.content))
			require.ErrorContains(t, err, test.message)
			assert.Nil(t, names, "failure must not return a partial replacement")
		})
	}
	for _, content := range []string{
		strings.Repeat("x", maxNameLineBytes),
		strings.Repeat("a\n", maxNamesCount),
		strings.Repeat("\n", maxNamesBytes),
	} {
		_, err := namesFromLines(strings.NewReader(content))
		require.NoError(t, err, "exact limits must be accepted")
	}
	names, err := namesFromLines(strings.NewReader(" first.txt \r\n\nsecond.txt"))
	require.NoError(t, err)
	assert.Equal(t, []string{"first.txt", "second.txt"}, names)
}

func TestNamesFromLinesPropagatesReaderError(t *testing.T) {
	want := errors.New("read failed")
	names, err := namesFromLines(io.MultiReader(strings.NewReader("valid\n"), failedReader{want}))
	require.ErrorIs(t, err, want)
	assert.Nil(t, names)
}

func namingDraft(a *App) *AppState {
	a.state.NewNames = []string{"keep.txt"}
	a.state.LoadedNames = []string{"loaded.txt"}
	a.state.Previews = []domain.RenamePreview{{OriginalName: "old.txt", NewName: "keep.txt"}}
	a.state.NamingMethod = "manual"
	a.state.AppliedMethod = "manual"
	a.state.NamesTyped = true
	a.state.ManualNames["/old.txt"] = "keep.txt"
	return a.snapshot().state
}

func assertDraftPreserved(t *testing.T, before, after *AppState) {
	t.Helper()
	assert.Equal(t, before.NewNames, after.NewNames)
	assert.Equal(t, before.LoadedNames, after.LoadedNames)
	assert.Equal(t, before.Previews, after.Previews)
	assert.Equal(t, before.NamingMethod, after.NamingMethod)
	assert.Equal(t, before.AppliedMethod, after.AppliedMethod)
	assert.Equal(t, before.NamesTyped, after.NamesTyped)
	assert.Equal(t, before.ManualNames, after.ManualNames)
	assert.NotEmpty(t, after.Error)
}

func TestUploadFailurePreservesDraftAndCleansMultipartFiles(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	for _, content := range []string{
		"valid\n" + strings.Repeat("x", maxNameLineBytes+1),
		strings.Repeat("\n", maxNamesBytes+1),
		strings.Repeat("a\n", maxNamesCount+1),
		strings.Repeat("\n", maxNamesBytes+(128<<10)),
	} {
		a := newTestApp()
		before := namingDraft(a)
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("namesfile", "names.txt")
		require.NoError(t, err)
		_, err = io.WriteString(part, content)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		req := httptest.NewRequest("POST", "/api/names/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		a.GetHandler().ServeHTTP(httptest.NewRecorder(), req)
		assertDraftPreserved(t, before, a.snapshot().state)
		files, err := os.ReadDir(temp)
		require.NoError(t, err)
		assert.Empty(t, files, "temporary multipart files must be removed on errors")
	}
}

type openedNamesFS struct {
	port.FileSystem
	open func(string) (io.ReadCloser, error)
}

func (f *openedNamesFS) OpenFile(path string) (io.ReadCloser, error) { return f.open(path) }

type trackedNamesReader struct {
	io.Reader
	closed bool
}

func (r *trackedNamesReader) Close() error { r.closed = true; return nil }

func TestDroppedNamesStreamIsBoundedAndPreservesDraftOnFailure(t *testing.T) {
	for _, reader := range []io.Reader{
		strings.NewReader("valid\n" + strings.Repeat("x", maxNameLineBytes+1)),
		strings.NewReader(strings.Repeat("\n", maxNamesBytes+1)),
		io.MultiReader(strings.NewReader("valid\n"), failedReader{errors.New("disk read failed")}),
	} {
		a := newTestApp()
		before := namingDraft(a)
		stream := &trackedNamesReader{Reader: reader}
		a.fs = &openedNamesFS{FileSystem: a.fs, open: func(string) (io.ReadCloser, error) {
			return stream, nil
		}}
		a.DropFiles([]string{"/names.txt"}, DropTargetNames)
		assertDraftPreserved(t, before, a.snapshot().state)
		assert.True(t, stream.closed)
	}
}

func TestLegacyDroppedNamesRejectsKnownOversizeBeforeRead(t *testing.T) {
	a := newTestApp()
	before := namingDraft(a)
	a.fs = &mockFS{
		StatFunc: func(string) (os.FileInfo, error) {
			return &mockFileInfo{size: maxNamesBytes + 1}, nil
		},
		ReadFileFunc: func(string) ([]byte, error) {
			t.Fatal("oversize file must not be read")
			return nil, nil
		},
	}
	a.DropFiles([]string{"/names.txt"}, DropTargetNames)
	assertDraftPreserved(t, before, a.snapshot().state)
}

func TestHTTPDropUsesNativeSemantics(t *testing.T) {
	a := dropTestApp()
	body := renderPost(t, a, "/api/drop", map[string][]string{
		"path": {"/tmp/names.txt"}, "target": {DropTargetNames},
	})
	assert.Contains(t, body, `id="names-editor"`)
	assert.Equal(t, []string{"first.txt", "second.txt"}, a.snapshot().state.NewNames)
}

func TestUploadReaderFailurePreservesDraft(t *testing.T) {
	a := newTestApp()
	before := namingDraft(a)
	var prefix bytes.Buffer
	writer := multipart.NewWriter(&prefix)
	part, err := writer.CreateFormFile("namesfile", "names.txt")
	require.NoError(t, err)
	_, err = io.WriteString(part, "valid\n")
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/api/names/upload",
		io.MultiReader(&prefix, failedReader{errors.New("upload interrupted")}))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	a.GetHandler().ServeHTTP(httptest.NewRecorder(), req)
	assertDraftPreserved(t, before, a.snapshot().state)
}

func TestSuccessfulUploadRemovesTemporaryFiles(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	a := newTestApp()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("namesfile", "names.txt")
	require.NoError(t, err)
	_, err = io.WriteString(part, strings.Repeat("a\n", 40000))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest("POST", "/api/names/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	a.GetHandler().ServeHTTP(httptest.NewRecorder(), req)
	assert.Len(t, a.snapshot().state.NewNames, 40000)
	files, err := os.ReadDir(temp)
	require.NoError(t, err)
	assert.Empty(t, files)
}
