package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/omegaatt36/dub/internal/port"
)

// Option configures the App.
type Option func(*App)

// WithLogger sets a custom logger for the App.
func WithLogger(logger *slog.Logger) Option {
	return func(a *App) {
		a.logger = logger
	}
}

// WithDirectoryPicker supplies the native directory chooser. The desktop shell
// owns the platform dialog and injects it here, which keeps the windowing
// runtime out of this package and lets tests bring their own picker.
func WithDirectoryPicker(pick func() (string, error)) Option {
	return func(a *App) {
		a.pickDirectory = pick
	}
}

// WithHomeDir sets the folder the tree is rooted at. Without it the tree only
// shows the directory the user opened.
func WithHomeDir(dir string) Option {
	return func(a *App) {
		a.home = filepath.Clean(dir)
	}
}

// Drop zones the frontend marks with the data-file-drop-target attribute. The
// value is what a drop there means; Wails reports it back with the drop.
const (
	DropTargetScan  = "scan"
	DropTargetNames = "names"
)

// App is the main application struct that composes all services.
type App struct {
	// operations orders HTTP mutations and native drops. mu protects only the
	// published snapshot; filesystem work and rendering use a private copy.
	operations    sync.Mutex
	mu            sync.Mutex
	fs            port.FileSystem
	scanner       port.Scanner
	pattern       port.PatternFilter
	renamer       port.Renamer
	state         *AppState
	pickDirectory func() (string, error)
	home          string
	logger        *slog.Logger
	// folderCache holds the folder tree's directory listings. See listFolders.
	folderCache map[string]folderListing
	// readOnly snapshots never discover folders while rendering a GET.
	readOnly bool
}

// NewApp creates a new App with injected service dependencies.
func NewApp(fs port.FileSystem, scanner port.Scanner, pattern port.PatternFilter, renamer port.Renamer, opts ...Option) *App {
	a := &App{
		fs:      fs,
		scanner: scanner,
		pattern: pattern,
		renamer: renamer,
		state:   NewAppState(),
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// GetHandler returns the HTTP handler for the HTMX API.
func (a *App) GetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutation := r.Method != http.MethodGet && r.Method != http.MethodHead
		if mutation {
			a.operations.Lock()
			defer a.operations.Unlock()
		}
		work := a.snapshot()
		work.readOnly = !mutation
		work.newRouter().ServeHTTP(w, r)
		if mutation {
			a.publish(work)
		}
	})
}

// OpenDirectoryDialog opens a native OS directory picker and returns the selected path.
func (a *App) OpenDirectoryDialog() (string, error) {
	if a.pickDirectory == nil {
		return "", errors.New("no directory picker configured")
	}
	return a.pickDirectory()
}

// DropFiles applies a desktop file drop to the app state. The zone the user
// dropped on decides the action: a .txt or .csv on the names editor replaces
// the manual name list, anywhere else scans the dropped path's directory.
func (a *App) DropFiles(paths []string, dropTarget string) {
	a.operations.Lock()
	defer a.operations.Unlock()
	work := a.snapshot()
	work.dropFiles(paths, dropTarget)
	// Warm newly revealed nodes before publication; GET only uses cached IO.
	work.folderRoots()
	a.publish(work)
}

func (a *App) dropFiles(paths []string, dropTarget string) {
	if len(paths) == 0 {
		return
	}
	path := paths[0]

	if dropTarget == DropTargetNames && isNamesFile(path) {
		a.loadNamesPath(path)
		return
	}
	a.requestScan(path)

	// Dub works on one folder at a time. Saying so beats silently ignoring the
	// rest of what the user dropped.
	if len(paths) > 1 {
		a.state.Error = fmt.Sprintf("%d items were dropped; opened only %s", len(paths), filepath.Base(path))
	}
}

// isNamesFile reports whether path is one of the name lists the manual editor
// accepts. A folder dropped on that zone is not a list, so it falls through to
// a scan like any other drop.
func isNamesFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".csv":
		return true
	default:
		return false
	}
}
