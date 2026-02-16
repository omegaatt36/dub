package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/mock"
)

func TestHandleScan_WithServiceMock(t *testing.T) {
	ctrl := gomock.NewController(t)

	scanner := mock.NewMockScanner(ctrl)
	pattern := mock.NewMockPatternFilter(ctrl)
	renamer := mock.NewMockRenamer(ctrl)

	expectedFiles := []domain.FileItem{
		{Name: "file1.txt", Path: "/test/dir/file1.txt", Extension: ".txt", Size: 100},
		{Name: "file2.txt", Path: "/test/dir/file2.txt", Extension: ".txt", Size: 200},
	}

	scanner.EXPECT().Scan("/test/dir").Return(expectedFiles, nil)

	app := NewApp(scanner, pattern, renamer)
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

func TestHandlePattern_WithServiceMock(t *testing.T) {
	ctrl := gomock.NewController(t)

	scanner := mock.NewMockScanner(ctrl)
	patternSvc := mock.NewMockPatternFilter(ctrl)
	renamer := mock.NewMockRenamer(ctrl)

	allFiles := []domain.FileItem{
		{Name: "file1.txt", Extension: ".txt"},
		{Name: "file2.txt", Extension: ".txt"},
		{Name: "photo.jpg", Extension: ".jpg"},
	}
	matchedFiles := []domain.FileItem{
		{Name: "file1.txt", Extension: ".txt"},
		{Name: "file2.txt", Extension: ".txt"},
	}

	patternSvc.EXPECT().MatchFiles(allFiles, "file").Return(matchedFiles, nil)

	app := NewApp(scanner, patternSvc, renamer)
	app.state.AllFiles = allFiles

	handler := app.GetHandler()

	form := url.Values{"pattern": {"file"}}
	req := httptest.NewRequest("POST", "/api/pattern", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, app.state.MatchedFiles, 2)
}

func TestHandlePreview_WithServiceMock(t *testing.T) {
	ctrl := gomock.NewController(t)

	scanner := mock.NewMockScanner(ctrl)
	patternSvc := mock.NewMockPatternFilter(ctrl)
	renamer := mock.NewMockRenamer(ctrl)

	files := []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	names := []string{"renamed"}
	expectedPreviews := []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "renamed.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/renamed.txt"},
	}

	renamer.EXPECT().PreviewRename(files, names).Return(expectedPreviews, nil)

	app := NewApp(scanner, patternSvc, renamer)
	app.state.AllFiles = files
	app.state.MatchedFiles = files
	app.state.NewNames = names

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, app.state.Previews, 1)
	assert.Equal(t, "renamed.txt", app.state.Previews[0].NewName)
}

func TestHandleExecute_WithServiceMock(t *testing.T) {
	ctrl := gomock.NewController(t)

	scanner := mock.NewMockScanner(ctrl)
	patternSvc := mock.NewMockPatternFilter(ctrl)
	renamer := mock.NewMockRenamer(ctrl)

	previews := []domain.RenamePreview{
		{OriginalName: "a.txt", NewName: "renamed.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/renamed.txt"},
	}

	renamer.EXPECT().ExecuteRename(previews).Return(domain.RenameResult{Success: true, Message: "Renamed 1 files", RenamedCount: 1})

	refreshedFiles := []domain.FileItem{
		{Name: "renamed.txt", Path: "/dir/renamed.txt", Extension: ".txt", Size: 100},
	}
	scanner.EXPECT().Scan("/dir").Return(refreshedFiles, nil)

	app := NewApp(scanner, patternSvc, renamer)
	app.state.SelectedDirectory = "/dir"
	app.state.AllFiles = []domain.FileItem{
		{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
	}
	app.state.MatchedFiles = app.state.AllFiles
	app.state.Previews = previews

	handler := app.GetHandler()

	req := httptest.NewRequest("POST", "/api/execute", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, app.state.Previews)
	assert.Len(t, app.state.AllFiles, 1)
}
