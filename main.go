package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

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

	application := app.NewApp(fileSystem, scanner, pattern, renamer)

	err := wails.Run(&options.App{
		Title:  "Dub",
		Width:  1200,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: application.GetHandler(),
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
			CSSDropProperty:    "--wails-drop-target",
			CSSDropValue:       "drop",
		},
		OnStartup:  application.Startup,
		OnShutdown: application.Shutdown,
	})
	if err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}
