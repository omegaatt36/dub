package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

type blockingScanner struct {
	port.Scanner
	scan func(string) ([]domain.FileItem, error)
	list func(string) ([]domain.FolderItem, error)
}

func (s *blockingScanner) Scan(path string) ([]domain.FileItem, error) {
	if s.scan != nil {
		return s.scan(path)
	}
	return s.Scanner.Scan(path)
}

func (s *blockingScanner) ListFolders(path string) ([]domain.FolderItem, error) {
	if s.list != nil {
		return s.list(path)
	}
	return s.Scanner.ListFolders(path)
}

type blockingRenamer struct {
	port.Renamer
	execute func([]domain.RenamePreview) domain.RenameResult
}

func (r *blockingRenamer) ExecuteRename(previews []domain.RenamePreview) domain.RenameResult {
	return r.execute(previews)
}

func requestAsync(handler http.Handler, method, path string, form url.Values) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		done <- rec
	}()
	return done
}

func awaitResponse(t *testing.T, done <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-done:
		return rec
	case <-time.After(3 * time.Second):
		t.Fatal("request did not complete")
		return nil
	}
}

func TestViewsRemainResponsiveDuringFilesystemOperations(t *testing.T) {
	for _, operation := range []string{"scan", "execute", "undo", "folders", "drop"} {
		t.Run(operation, func(t *testing.T) {
			a := newTestApp()
			a.state.SelectedDirectory = "/old"
			a.state.AllFiles = []domain.FileItem{{Name: "old.txt", Path: "/old/old.txt"}}
			a.state.MatchedFiles = a.state.AllFiles
			a.state.Selected["/old/old.txt"] = true
			a.state.Previews = []domain.RenamePreview{{
				OriginalName: "old.txt", NewName: "new.txt",
				OriginalPath: "/old/old.txt", NewPath: "/old/new.txt",
			}}
			a.state.LastRenameHistory = a.state.Previews
			a.state.LastRenameDir = "/old"
			a.state.CanUndo = true
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			block := func() { close(started); <-release }
			route := "/api/scan"
			form := url.Values{"path": {"/new"}}
			switch operation {
			case "scan", "drop":
				a.scanner = &blockingScanner{Scanner: a.scanner, scan: func(string) ([]domain.FileItem, error) {
					block()
					return []domain.FileItem{{Name: "new.txt", Path: "/new/new.txt"}}, nil
				}}
			case "execute", "undo":
				route = "/api/" + operation
				a.renamer = &blockingRenamer{Renamer: a.renamer, execute: func([]domain.RenamePreview) domain.RenameResult {
					block()
					return domain.RenameResult{Success: true, RenamedCount: 1}
				}}
			case "folders":
				route = "/api/tree/toggle"
				a.scanner = &blockingScanner{Scanner: a.scanner, list: func(string) ([]domain.FolderItem, error) {
					block()
					return nil, nil
				}}
			}
			handler := a.GetHandler()
			var done <-chan *httptest.ResponseRecorder
			if operation == "drop" {
				finished := make(chan *httptest.ResponseRecorder, 1)
				go func() {
					a.DropFiles([]string{"/new"}, DropTargetScan)
					finished <- httptest.NewRecorder()
				}()
				done = finished
			} else {
				done = requestAsync(handler, "POST", route, form)
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("filesystem operation did not start")
			}

			// A later mutation cannot commit against an older snapshot.
			queued := requestAsync(handler, "POST", "/api/hidden-files", url.Values{"show": {"true"}})
			select {
			case <-queued:
				t.Fatal("mutation bypassed the operation gate")
			case <-time.After(20 * time.Millisecond):
			}
			for _, view := range []string{"/api/page", "/api/main"} {
				rec := awaitResponse(t, requestAsync(handler, "GET", view, nil))
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Contains(t, rec.Body.String(), "old.txt")
			}
			unblock()
			awaitResponse(t, done)
			awaitResponse(t, queued)
			assert.True(t, a.snapshot().state.ShowHiddenFiles, "queued mutation must not be overwritten by scan completion")
		})
	}
}

func TestNativeDropWaitsForEarlierMutation(t *testing.T) {
	a := newTestApp()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	a.scanner = &blockingScanner{Scanner: a.scanner, scan: func(path string) ([]domain.FileItem, error) {
		if path == "/first" {
			close(started)
			<-release
		}
		return nil, nil
	}}
	done := requestAsync(a.GetHandler(), "POST", "/api/scan", url.Values{"path": {"/first"}})
	<-started
	dropped := make(chan struct{})
	go func() {
		a.DropFiles([]string{"/second"}, DropTargetScan)
		close(dropped)
	}()
	select {
	case <-dropped:
		t.Fatal("native drop bypassed an earlier scan")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	awaitResponse(t, done)
	select {
	case <-dropped:
	case <-time.After(3 * time.Second):
		t.Fatal("drop did not complete")
	}
	assert.Equal(t, "/second", a.snapshot().state.SelectedDirectory)
}

func TestSnapshotOwnsAllMutableCollections(t *testing.T) {
	a := newTestApp()
	a.state.AllFiles = []domain.FileItem{{Name: "all"}}
	a.state.MatchedFiles = a.state.AllFiles
	a.state.Selected["path"] = true
	a.state.ManualNames["path"] = "manual"
	a.state.Expanded["path"] = true
	a.state.NewNames = []string{"new"}
	a.state.LoadedNames = []string{"loaded"}
	a.state.Previews = []domain.RenamePreview{{NewName: "preview"}}
	a.state.LastRenameHistory = a.state.Previews
	a.state.ExtraRoots = []string{"root"}
	a.folderCache = map[string]folderListing{"path": {folders: []domain.FolderItem{{Name: "folder"}}}}
	work := a.snapshot()
	work.state.AllFiles[0].Name = "changed"
	work.state.MatchedFiles[0].Name = "changed"
	delete(work.state.Selected, "path")
	work.state.ManualNames["path"] = "changed"
	delete(work.state.Expanded, "path")
	work.state.NewNames[0] = "changed"
	work.state.LoadedNames[0] = "changed"
	work.state.Previews[0].NewName = "changed"
	work.state.LastRenameHistory[0].NewName = "changed"
	work.state.ExtraRoots[0] = "changed"
	work.folderCache["path"].folders[0].Name = "changed"
	assert.Equal(t, "all", a.state.AllFiles[0].Name)
	assert.Equal(t, "all", a.state.MatchedFiles[0].Name)
	assert.True(t, a.state.Selected["path"])
	assert.Equal(t, "manual", a.state.ManualNames["path"])
	assert.True(t, a.state.Expanded["path"])
	assert.Equal(t, "new", a.state.NewNames[0])
	assert.Equal(t, "loaded", a.state.LoadedNames[0])
	assert.Equal(t, "preview", a.state.Previews[0].NewName)
	assert.Equal(t, "preview", a.state.LastRenameHistory[0].NewName)
	assert.Equal(t, "root", a.state.ExtraRoots[0])
	assert.Equal(t, "folder", a.folderCache["path"].folders[0].Name)
}

func TestConcurrentViewsAndMutations(t *testing.T) {
	a := newTestApp()
	renderPost(t, a, "/api/scan", url.Values{"path": {"/dir"}})
	handler := a.GetHandler()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 20 {
				req := httptest.NewRequest("GET", "/api/page", nil)
				handler.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
	for range 20 {
		awaitResponse(t, requestAsync(handler, "POST", "/api/tick-all", url.Values{"ticked": {"false"}}))
		awaitResponse(t, requestAsync(handler, "POST", "/api/tick-all", url.Values{"ticked": {"true"}}))
	}
	wg.Wait()
	require.Len(t, a.snapshot().state.Selected, 2)
}
