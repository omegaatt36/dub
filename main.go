package main

import (
	"embed"
	"log/slog"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/omegaatt36/dub/app"
	"github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/service"
)

// The frontend is embedded straight from web/ so a fresh clone builds with no
// generated copy of the tree.
//
// Only the HTML shell and the static tree are embedded. web/template holds the
// templ sources and their generated Go files, and the asset server serves
// whatever the fs.FS contains, so embedding that directory would publish the
// application source at /template/page_templ.go.
//
// The asset server roots the fs.FS at the directory holding index.html, so "/"
// resolves to web/index.html and "/static/..." to web/static/... without any
// further wiring.
var (
	//go:embed web/index.html
	//go:embed all:web/static
	assets embed.FS
)

// dropTargetAttribute is the HTML attribute the frontend marks its drop zones
// with. Wails echoes it back on the drop event, and its value is what the drop
// means.
const dropTargetAttribute = "data-file-drop-target"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	fileSystem := &fs.OSFileSystem{}
	patternMatcher := &regex.Engine{}

	scanner := service.NewScannerService(fileSystem)
	pattern := service.NewPatternService(patternMatcher)
	renamer := service.NewRenamerService(fileSystem)

	dub := app.NewApp(fileSystem, scanner, pattern, renamer, app.WithDirectoryPicker(pickDirectory))

	desktop := application.New(application.Options{
		Name:        "Dub",
		Description: "Batch File Renamer",
		Assets: application.AssetOptions{
			Handler: newAssetHandler(dub.GetHandler()),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	win := desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Dub",
		Width:          1200,
		Height:         900,
		URL:            "/",
		EnableFileDrop: true,
	})

	// Wails routes the drop to Go, not to the page: a File object from the
	// webview has no path behind it. The drop zone decides the action, the app
	// applies it, and the page is asked to pull the result.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		files := e.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}

		target := ""
		if details := e.Context().DropTargetDetails(); details != nil {
			target = details.Attributes[dropTargetAttribute]
		}

		dub.DropFiles(files, target)
		win.ExecJS("window.dubRefresh()")
	})

	err := desktop.Run()
	if err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}

// newAssetHandler composes everything the webview can ask for: the embedded
// frontend at the root, and the HTMX API under /api/. ServeMux matches the
// longer /api/ pattern first, so the two never depend on registration order.
func newAssetHandler(api http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/", application.BundledAssetFileServer(assets))
	return mux
}

// pickDirectory shows the platform directory chooser. Wails serves the dialog
// synchronously over the calling goroutine, which is why it runs from the HTTP
// handler goroutine and never while the app state lock is held.
func pickDirectory() (string, error) {
	return application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                "Select Directory",
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	}).PromptForSingleSelection()
}
