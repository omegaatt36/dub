package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

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

// App is the main application struct that composes all services.
type App struct {
	mu      sync.Mutex
	fs      port.FileSystem
	scanner port.Scanner
	pattern port.PatternFilter
	renamer port.Renamer
	state   *AppState
	ctx     context.Context
	logger  *slog.Logger
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

// GetHandler returns the Chi HTTP handler for the asset server.
func (a *App) GetHandler() http.Handler {
	return a.newRouter()
}

// Startup is called when the Wails app starts. It stores the runtime context.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// Shutdown is called when the Wails app is closing.
func (a *App) Shutdown(_ context.Context) {
}

// OpenDirectoryDialog opens a native OS directory picker and returns the selected path.
func (a *App) OpenDirectoryDialog() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Directory",
	})
}
