package app

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/a-h/templ"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
	"github.com/omegaatt36/dub/web/template"
)

func (a *App) newRouter() http.Handler {
	mux := http.NewServeMux()

	// HTMX routes — static files are served by Wails AssetServer directly
	mux.HandleFunc("GET /api/page", a.handlePage)
	mux.HandleFunc("POST /api/select-directory", a.handleSelectDirectory)
	mux.HandleFunc("POST /api/scan", a.handleScan)
	mux.HandleFunc("POST /api/tick", a.handleTick)
	mux.HandleFunc("POST /api/tick-all", a.handleTickAll)
	mux.HandleFunc("POST /api/pattern", a.handlePattern)
	mux.HandleFunc("POST /api/extension-policy", a.handleExtensionPolicy)
	mux.HandleFunc("POST /api/names", a.handleNames)
	mux.HandleFunc("POST /api/names/generate", a.handleNamesGenerate)
	mux.HandleFunc("POST /api/names/findreplace", a.handleNamesFindReplace)
	mux.HandleFunc("POST /api/names/upload", a.handleNamesUpload)
	mux.HandleFunc("POST /api/preview", a.handlePreview)
	mux.HandleFunc("POST /api/execute", a.handleExecute)
	mux.HandleFunc("POST /api/undo", a.handleUndo)
	mux.HandleFunc("POST /api/names/load", a.handleNamesLoad)

	// No CORS headers: the UI is served from this same origin, so htmx
	// requests are same-origin and need none. Advertising a wildcard origin
	// would let any page the user visits drive /api/execute and rename files.
	// Per-request logging is omitted on purpose; handlers log the events that
	// matter (scan, rename, undo) through a.logger.
	return mux
}

// handlePage returns the inner page content (no HTML shell).
// index.html is the shell; this endpoint provides the dynamic body.
func (a *App) handlePage(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	data := a.buildPageData(nil)
	renderTempl(w, r, template.AppContent(data))
}

func (a *App) handleSelectDirectory(w http.ResponseWriter, r *http.Request) {
	// The native dialog blocks until the user picks or cancels, so it runs
	// without the state lock. Holding it would stall every other request.
	path, err := a.OpenDirectoryDialog()

	a.mu.Lock()
	defer a.mu.Unlock()

	if err != nil || path == "" {
		// User cancelled the dialog or error — return current state unchanged
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.SelectedDirectory = path
	a.state.ResetForDirectory()

	files, err := a.scanner.Scan(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to scan directory: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.AllFiles = files
	a.state.MatchedFiles = files
	a.state.SelectAllTicks(files)
	a.state.Error = ""

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleScan(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := r.FormValue("path")
	if path == "" {
		a.state.Error = "No directory path provided"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	// If path is a file, use its parent directory
	if info, err := a.fs.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}

	a.state.SelectedDirectory = path
	a.state.ResetForDirectory()

	files, err := a.scanner.Scan(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to scan directory: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.AllFiles = files
	a.state.MatchedFiles = files
	a.state.SelectAllTicks(files)
	a.state.Error = ""
	a.logger.Info("directory scanned", "path", path, "file_count", len(files))

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleTick toggles one file's checkbox. Changing the ticked set invalidates
// the current names and previews because they were computed for a different
// set of files.
func (a *App) handleTick(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := r.FormValue("path")
	if path == "" {
		a.state.Error = "No file path provided"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	if r.FormValue("ticked") == "true" {
		a.state.Selected[path] = true
	} else {
		delete(a.state.Selected, path)
	}

	a.state.ResetForPattern()
	a.state.Error = ""
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleTickAll ticks or unticks every file currently visible in the list.
func (a *App) handleTickAll(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.FormValue("ticked") == "true" {
		a.state.SelectAllTicks(a.visibleFiles())
	} else {
		clear(a.state.Selected)
	}

	a.state.ResetForPattern()
	a.state.Error = ""
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handlePattern(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	pattern := r.FormValue("pattern")
	a.state.Pattern = pattern
	a.state.ResetForPattern()

	a.applyFilter()
	a.state.Error = ""

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleExtensionPolicy toggles whether a rename may rewrite a file's
// extension. Changing it invalidates the current previews because it changes
// what every proposed name resolves to.
func (a *App) handleExtensionPolicy(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.FormValue("convert") == "true" {
		a.state.ExtensionPolicy = port.UseProposedExtension
	} else {
		a.state.ExtensionPolicy = port.KeepExtension
	}

	a.state.ResetForPattern()
	a.state.Error = ""
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNames(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	method := r.FormValue("method")
	if method != "" {
		a.state.NamingMethod = method
	}
	action := r.FormValue("action")

	if action == "update" {
		// Collect names from form and return full content so Actions updates
		files := a.activeFiles()
		names := make([]string, len(files))
		for i := range files {
			names[i] = r.FormValue(fmt.Sprintf("name_%d", i))
		}
		a.state.NewNames = names
		a.autoPreview()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	// Method toggle only — just swap the editor panel
	renderTempl(w, r, template.NamesEditor(template.EditorData{
		Files:          a.activeFiles(),
		Names:          a.state.NewNames,
		Method:         a.state.NamingMethod,
		Template:       a.state.Template,
		Search:         a.state.SearchPattern,
		Replace:        a.state.ReplacePattern,
		AllowExtChange: a.state.ExtensionPolicy == port.UseProposedExtension,
	}))
}

func (a *App) handleNamesGenerate(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	tmpl := r.FormValue("template")
	if tmpl == "" {
		tmpl = "name_{index}"
	}
	a.state.Template = tmpl

	files := a.activeFiles()
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = domain.ExpandTemplate(tmpl, f, i)
	}
	a.state.NewNames = names
	a.state.NamingMethod = "template"
	a.autoPreview()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesFindReplace(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	search := r.FormValue("search")
	replace := r.FormValue("replace")

	a.state.SearchPattern = search
	a.state.ReplacePattern = replace
	a.state.NamingMethod = "findreplace"

	files := a.activeFiles()
	names, err := domain.FindReplace(files, search, replace)
	if err != nil {
		a.state.Error = fmt.Sprintf("Invalid search pattern: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.NewNames = names
	a.state.Error = ""
	a.autoPreview()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesUpload(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	file, _, err := r.FormFile("namesfile")
	if err != nil {
		a.state.Error = "Failed to read uploaded file"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	defer func() {
		_ = file.Close()
	}()

	content, err := io.ReadAll(file)
	if err != nil {
		a.state.Error = "Failed to read file content"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			names = append(names, line)
		}
	}

	a.state.NewNames = names
	a.state.NamingMethod = "file"
	a.autoPreview()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleNamesLoad reads a names file by path (for drag & drop).
func (a *App) handleNamesLoad(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := r.FormValue("path")
	if path == "" {
		a.state.Error = "No file path provided"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	content, err := a.fs.ReadFile(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to read file: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			names = append(names, line)
		}
	}

	a.state.NewNames = names
	a.state.NamingMethod = "file"
	a.autoPreview()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handlePreview(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.FormValue("clear") == "true" {
		a.state.NewNames = nil
		a.state.ClearPreviews()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	files := a.activeFiles()
	previews, err := a.renamer.PreviewRename(files, a.state.NewNames, a.state.ExtensionPolicy)
	if err != nil {
		a.state.Error = fmt.Sprintf("Preview failed: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.Previews = previews
	a.state.Error = ""

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleExecute(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.state.Previews) == 0 {
		a.state.Error = "No previews to execute"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	result := a.renamer.ExecuteRename(a.state.Previews)

	// Save undo history before resetting state
	if result.Success {
		a.state.LastRenameHistory = make([]domain.RenamePreview, len(a.state.Previews))
		copy(a.state.LastRenameHistory, a.state.Previews)
		a.state.CanUndo = true
	}

	a.logger.Info("rename executed", "renamed_count", result.RenamedCount, "error_count", len(result.Errors))
	a.state.ResetForExecute()

	// Re-scan the directory to refresh the file list. The filter survives the
	// execute, so it has to be re-applied to the refreshed files.
	a.refreshFiles()

	renderTempl(w, r, template.MainContent(a.buildPageData(&result)))
}

func (a *App) handleUndo(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.state.CanUndo || len(a.state.LastRenameHistory) == 0 {
		a.state.Error = "Nothing to undo"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	// Build reversed previews: swap Original <-> New
	reversed := make([]domain.RenamePreview, len(a.state.LastRenameHistory))
	for i, p := range a.state.LastRenameHistory {
		reversed[i] = domain.RenamePreview{
			OriginalName: p.NewName,
			NewName:      p.OriginalName,
			OriginalPath: p.NewPath,
			NewPath:      p.OriginalPath,
		}
	}

	result := a.renamer.ExecuteRename(reversed)
	a.state.CanUndo = false
	a.state.LastRenameHistory = nil

	a.logger.Info("undo executed", "restored_count", result.RenamedCount, "error_count", len(result.Errors))

	a.refreshFiles()

	renderTempl(w, r, template.MainContent(a.buildPageData(&result)))
}

// applyFilter recomputes MatchedFiles from the current Pattern.
// An invalid pattern is not fatal: the view falls back to the full list and
// reports the reason through PatternError.
// Callers must hold a.mu.
func (a *App) applyFilter() {
	if a.state.Pattern == "" {
		a.state.MatchedFiles = a.state.AllFiles
		return
	}

	matched, err := a.pattern.MatchFiles(a.state.AllFiles, a.state.Pattern)
	if err != nil {
		a.state.PatternError = err.Error()
		a.state.MatchedFiles = a.state.AllFiles
		return
	}

	a.state.PatternError = ""
	a.state.MatchedFiles = matched
}

// refreshFiles re-scans the selected directory and re-applies the filter so the
// user keeps looking at the same subset after files were renamed or restored.
// A failed scan leaves the previous list in place.
// Callers must hold a.mu.
func (a *App) refreshFiles() {
	if a.state.SelectedDirectory == "" {
		return
	}

	files, err := a.scanner.Scan(a.state.SelectedDirectory)
	if err != nil {
		a.logger.Warn("rescan failed", "path", a.state.SelectedDirectory, "error", err)
		return
	}

	a.state.AllFiles = files
	// Renaming changes paths, so every tick held before the rename is now
	// dangling. Re-ticking the refreshed list keeps "rename everything" the
	// default without letting stale paths accumulate.
	a.state.SelectAllTicks(files)
	a.applyFilter()
}

// autoPreview generates previews automatically when names are available.
func (a *App) autoPreview() {
	files := a.activeFiles()
	if len(a.state.NewNames) == 0 || len(files) == 0 {
		a.state.Previews = nil
		return
	}

	previews, err := a.renamer.PreviewRename(files, a.state.NewNames, a.state.ExtensionPolicy)
	if err != nil {
		a.state.Error = fmt.Sprintf("Preview failed: %v", err)
		a.state.Previews = nil
		return
	}

	a.state.Previews = previews
}

func (a *App) buildPageData(result *domain.RenameResult) template.PageData {
	visible := a.visibleFiles()
	active := a.activeFiles()
	return template.PageData{
		SelectedDirectory: a.state.SelectedDirectory,
		VisibleFiles:      visible,
		ActiveFiles:       active,
		SelectedPaths:     a.state.Selected,
		AllFiles:          a.state.AllFiles,
		MatchedFiles:      a.state.MatchedFiles,
		TickedCount:       a.state.TickCount(visible),
		ConflictCount:     countConflicts(a.state.Previews),
		Pattern:           a.state.Pattern,
		PatternError:      a.state.PatternError,
		NewNames:          a.state.NewNames,
		Previews:          a.state.Previews,
		AllowExtChange:    a.state.ExtensionPolicy == port.UseProposedExtension,
		ExtChangeCount:    countExtensionChanges(a.state.Previews),
		Error:             a.state.Error,
		NamingMethod:      a.state.NamingMethod,
		Template:          a.state.Template,
		SearchPattern:     a.state.SearchPattern,
		ReplacePattern:    a.state.ReplacePattern,
		CanUndo:           a.state.CanUndo,
		Result:            result,
	}
}

// visibleFiles is what the file list renders: the pattern-filtered subset when a
// pattern is active, otherwise every scanned file. Ticking does not affect it,
// so unticked files stay visible (and re-tickable) instead of vanishing.
func (a *App) visibleFiles() []domain.FileItem {
	if a.state.Pattern != "" {
		return a.state.MatchedFiles
	}
	return a.state.AllFiles
}

// activeFiles is the set a rename actually applies to: visible AND ticked.
// Every naming method and the execute path read this, never visibleFiles, so a
// rename can never touch a file the user did not choose.
func (a *App) activeFiles() []domain.FileItem {
	visible := a.visibleFiles()
	active := make([]domain.FileItem, 0, len(visible))
	for _, f := range visible {
		if a.state.Selected[f.Path] {
			active = append(active, f)
		}
	}
	return active
}

// countExtensionChanges reports how many previews rewrite the extension, so the
// UI can warn about the real blast radius rather than the policy alone.
func countExtensionChanges(previews []domain.RenamePreview) int {
	n := 0
	for _, p := range previews {
		if filepath.Ext(p.OriginalName) != filepath.Ext(p.NewName) {
			n++
		}
	}
	return n
}

func countConflicts(previews []domain.RenamePreview) int {
	n := 0
	for _, p := range previews {
		if p.Conflict {
			n++
		}
	}
	return n
}

func renderTempl(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = component.Render(r.Context(), w)
}
