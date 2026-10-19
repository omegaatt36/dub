package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
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
	mux.HandleFunc("GET /api/main", a.handleMain)
	mux.HandleFunc("POST /api/select-directory", a.handleSelectDirectory)
	mux.HandleFunc("POST /api/scan", a.handleScan)
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

// handleMain returns the content pane only. A desktop drop has no request to
// render from, so the frontend pulls the new state through here instead.
func (a *App) handleMain(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
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

	a.scanPath(path)
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleScan(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := r.FormValue("path")
	if path == "" {
		a.state.Error = "No directory path provided"
	} else {
		a.scanPath(path)
	}

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// scanPath points the app at path and loads its files. A file path is narrowed
// to its parent directory, because a desktop drop can hand over either a
// folder or a single file. Callers hold a.mu.
func (a *App) scanPath(path string) {
	if info, err := a.fs.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}

	a.state.SelectedDirectory = path
	a.state.ResetForDirectory()

	files, err := a.scanner.Scan(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to scan directory: %v", err)
		return
	}

	a.setScanned(files)
	a.state.Error = ""
	a.logger.Info("directory scanned", "path", path, "file_count", len(files))
}

// handleTick toggles one file's checkbox.
func (a *App) handleTick(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := r.FormValue("path")
	if path == "" {
		a.state.Error = "No file path provided"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	ticked := r.FormValue("ticked") == "true"
	a.state.Error = ""
	a.changeFileSet(func() {
		if ticked {
			a.state.Selected[path] = true
		} else {
			delete(a.state.Selected, path)
		}
	})
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleTickAll ticks or unticks every file currently visible in the list.
func (a *App) handleTickAll(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ticked := r.FormValue("ticked") == "true"
	a.state.Error = ""
	a.changeFileSet(func() {
		if ticked {
			a.state.SelectAllTicks(a.visibleFiles())
		} else {
			clear(a.state.Selected)
		}
	})
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleHiddenFiles changes whether dotfiles are listed. Files that were hidden
// are not ticked by revealing them: a rename must only touch files the user
// chose, and the user has not seen these yet. The preference lasts for the
// current session.
func (a *App) handleHiddenFiles(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	show := r.FormValue("show") == "true"
	a.state.Error = ""
	a.changeFileSet(func() {
		a.state.ShowHiddenFiles = show
		a.applyFilter()
	})
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleSort reorders the file list. Order decides {index} and the position of
// every manual name, so it is a file-set change like any other.
func (a *App) handleSort(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	order, err := domain.ParseSortOrder(r.FormValue("sort"))
	if err != nil {
		a.state.Error = err.Error()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.Error = ""
	a.changeFileSet(func() {
		a.state.Sort = order
		domain.SortFiles(a.state.AllFiles, order)
		a.applyFilter()
	})
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handlePattern(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	pattern := r.FormValue("pattern")
	a.state.Error = ""
	a.changeFileSet(func() {
		a.state.Pattern = pattern
		a.applyFilter()
	})
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleExtensionPolicy toggles whether a rename may rewrite a file's
// extension. The names stay; only what they resolve to changes, so the previews
// are rebuilt from them.
func (a *App) handleExtensionPolicy(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.FormValue("convert") == "true" {
		a.state.ExtensionPolicy = port.UseProposedExtension
	} else {
		a.state.ExtensionPolicy = port.KeepExtension
	}

	a.state.Error = ""
	a.autoPreview()
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

	a.state.NewNames = templateNames(a.activeFiles(), tmpl)
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

	a.state.NewNames = namesFromLines(strings.NewReader(string(content)))
	a.state.NamingMethod = "file"
	a.autoPreview()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// namesFromLines splits a names file into one name per non-empty line.
func namesFromLines(r io.Reader) []string {
	var names []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// handleNamesLoad reads a names file by path (for drag & drop).
func (a *App) handleNamesLoad(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := r.FormValue("path")
	if path == "" {
		a.state.Error = "No file path provided"
	} else {
		a.loadNamesPath(path)
	}

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// loadNamesPath replaces the manual name list with the non-empty lines of the
// file at path. Callers hold a.mu.
func (a *App) loadNamesPath(path string) {
	content, err := a.fs.ReadFile(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to read file: %v", err)
		return
	}

	a.state.NewNames = namesFromLines(bytes.NewReader(content))
	a.state.NamingMethod = "file"
	a.autoPreview()
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

// applyFilter recomputes MatchedFiles from the hidden-file rule and the current
// Pattern. It is the one place visibility is decided; everything else reads
// MatchedFiles. An invalid pattern is not fatal: the view falls back to the
// unfiltered list and reports the reason through PatternError.
// Callers must hold a.mu.
func (a *App) applyFilter() {
	files := a.state.AllFiles
	if !a.state.ShowHiddenFiles {
		files = slices.DeleteFunc(slices.Clone(files), domain.FileItem.IsHidden)
	}

	a.state.PatternError = ""
	if a.state.Pattern == "" {
		a.state.MatchedFiles = files
		return
	}

	matched, err := a.pattern.MatchFiles(files, a.state.Pattern)
	if err != nil {
		a.state.PatternError = err.Error()
		a.state.MatchedFiles = files
		return
	}

	a.state.MatchedFiles = matched
}

// setScanned installs a fresh scan: it applies the session sort, recomputes the
// visible list, and ticks what the user can see. Files hidden by the dotfile
// rule stay unticked until the user ticks them. Callers must hold a.mu.
func (a *App) setScanned(files []domain.FileItem) {
	domain.SortFiles(files, a.state.Sort)
	a.state.AllFiles = files
	a.state.PruneTicks(files)
	a.applyFilter()
	a.state.SelectAllTicks(a.state.MatchedFiles)
}

// changeFileSet runs mutate, which alters which files are listed, ticked or
// ordered, and carries the user's naming work over to the new set. A rename
// rule the user already applied is recomputed rather than thrown away, so
// unticking one file does not cost the user a trip back to "Apply".
// Callers must hold a.mu.
func (a *App) changeFileSet(mutate func()) {
	applied := len(a.state.NewNames) > 0
	a.rememberManualNames()
	mutate()
	if applied {
		a.rebuildNames()
	}
}

// rememberManualNames records the typed names against their files before the
// set changes, because NewNames is positional and would otherwise misattribute
// them as soon as one file leaves or the order moves.
func (a *App) rememberManualNames() {
	if a.state.NamingMethod != "manual" {
		return
	}
	files := a.activeFiles()
	if len(files) != len(a.state.NewNames) {
		return
	}
	for i, f := range files {
		a.state.ManualNames[f.Path] = a.state.NewNames[i]
	}
}

// rebuildNames recomputes NewNames and Previews for the current active files
// from whatever rule produced them. Names loaded from a file stay as they are:
// they are an ordered list the user supplied, not a rule.
// Callers must hold a.mu.
func (a *App) rebuildNames() {
	files := a.activeFiles()
	if len(files) == 0 {
		a.state.NewNames = nil
		a.state.Previews = nil
		return
	}

	switch a.state.NamingMethod {
	case "template":
		a.state.NewNames = templateNames(files, a.state.Template)
	case "findreplace":
		names, err := domain.FindReplace(files, a.state.SearchPattern, a.state.ReplacePattern)
		if err != nil {
			a.state.NewNames = nil
			a.state.Previews = nil
			return
		}
		a.state.NewNames = names
	case "manual":
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = a.state.ManualNames[f.Path]
		}
		a.state.NewNames = names
	}
	a.autoPreview()
}

func templateNames(files []domain.FileItem, tmpl string) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = domain.ExpandTemplate(tmpl, f, i)
	}
	return names
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

	// Renaming changes paths, so every tick held before the rename is now
	// dangling. Re-ticking the visible list keeps "rename everything" the
	// default without letting stale paths accumulate.
	a.setScanned(files)
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
	hidden := 0
	for _, f := range a.state.AllFiles {
		if f.IsHidden() {
			hidden++
		}
	}
	total := len(a.state.AllFiles)
	if !a.state.ShowHiddenFiles {
		total -= hidden
	}
	return template.PageData{
		SelectedDirectory: a.state.SelectedDirectory,
		VisibleFiles:      visible,
		ActiveFiles:       active,
		SelectedPaths:     a.state.Selected,
		ScannedCount:      len(a.state.AllFiles),
		TotalCount:        total,
		HiddenCount:       hidden,
		ShowHiddenFiles:   a.state.ShowHiddenFiles,
		SortOrder:         a.state.Sort.String(),
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

// visibleFiles is what the file list renders. Ticking does not affect it, so
// unticked files stay visible (and re-tickable) instead of vanishing.
func (a *App) visibleFiles() []domain.FileItem {
	return a.state.MatchedFiles
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
		if !strings.EqualFold(domain.Extension(p.OriginalName), domain.Extension(p.NewName)) {
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
