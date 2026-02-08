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
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	fileSystem := &fs.OSFileSystem{}
	patternMatcher := &regex.Engine{}
	application := app.NewApp(fileSystem, patternMatcher)

	err := wails.Run(&options.App{
		Title:  "Dub",
		Width:  1000,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: application.GetHandler(),
		},
		OnStartup:  application.Startup,
		OnShutdown: application.Shutdown,
	})
	if err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}
