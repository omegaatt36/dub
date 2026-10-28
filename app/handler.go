package app

import "net/http"

// newRouter binds handlers to a request's private snapshot. Public callers use
// GetHandler, which owns serialization and publication.
func (a *App) newRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/page", a.handlePage)
	mux.HandleFunc("GET /api/main", a.handleMain)
	mux.HandleFunc("POST /api/select-directory", a.handleSelectDirectory)
	mux.HandleFunc("POST /api/scan", a.handleScan)
	mux.HandleFunc("POST /api/scan/cancel", a.handleScanCancel)
	mux.HandleFunc("POST /api/tree/toggle", a.handleTreeToggle)
	mux.HandleFunc("POST /api/tree/visibility", a.handleTreeVisibility)
	mux.HandleFunc("POST /api/refresh", a.handleRefresh)
	mux.HandleFunc("POST /api/tick", a.handleTick)
	mux.HandleFunc("POST /api/tick-all", a.handleTickAll)
	mux.HandleFunc("POST /api/pattern", a.handlePattern)
	mux.HandleFunc("POST /api/hidden-files", a.handleHiddenFiles)
	mux.HandleFunc("POST /api/sort", a.handleSort)
	mux.HandleFunc("POST /api/extension-policy", a.handleExtensionPolicy)
	mux.HandleFunc("POST /api/names", a.handleNames)
	mux.HandleFunc("POST /api/names/generate", a.handleNamesGenerate)
	mux.HandleFunc("POST /api/names/findreplace", a.handleNamesFindReplace)
	mux.HandleFunc("POST /api/names/upload", a.handleNamesUpload)
	mux.HandleFunc("POST /api/names/load", a.handleNamesLoad)
	mux.HandleFunc("POST /api/preview", a.handlePreview)
	mux.HandleFunc("POST /api/execute", a.handleExecute)
	mux.HandleFunc("POST /api/undo", a.handleUndo)
	mux.HandleFunc("POST /api/recover", a.handleRecover)
	mux.HandleFunc("POST /api/drop", a.handleDrop)

	// No CORS headers: wildcard access would let unrelated pages rename files.
	return mux
}
