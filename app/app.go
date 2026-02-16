package app

import (
	"context"
	"net/http"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/omegaatt36/dub/internal/port"
)

// App is the main application struct that composes all services.
type App struct {
	scanner port.Scanner
	pattern port.PatternFilter
	renamer port.Renamer
	state   *AppState
	ctx     context.Context
}

// NewApp creates a new App with injected service dependencies.
func NewApp(scanner port.Scanner, pattern port.PatternFilter, renamer port.Renamer) *App {
	return &App{
		scanner: scanner,
		pattern: pattern,
		renamer: renamer,
		state:   NewAppState(),
	}
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
