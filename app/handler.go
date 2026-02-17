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
	"github.com/omegaatt36/dub/web/template"
)

func (a *App) newRouter() http.Handler {
	mux := http.NewServeMux()

	// HTMX routes — static files are served by Wails AssetServer directly
	mux.HandleFunc("GET /api/page", a.handlePage)
	mux.HandleFunc("POST /api/select-directory", a.handleSelectDirectory)
	mux.HandleFunc("POST /api/scan", a.handleScan)
	mux.HandleFunc("POST /api/pattern", a.handlePattern)
	mux.HandleFunc("POST /api/names", a.handleNames)
	mux.HandleFunc("POST /api/names/generate", a.handleNamesGenerate)
	mux.HandleFunc("POST /api/names/findreplace", a.handleNamesFindReplace)
	mux.HandleFunc("POST /api/names/upload", a.handleNamesUpload)
	mux.HandleFunc("POST /api/preview", a.handlePreview)
	mux.HandleFunc("POST /api/execute", a.handleExecute)
	mux.HandleFunc("POST /api/undo", a.handleUndo)
	mux.HandleFunc("POST /api/names/load", a.handleNamesLoad)

	return mux
}

// handlePage returns the inner page content (no HTML shell).
// index.html is the shell; this endpoint provides the dynamic body.
func (a *App) handlePage(w http.ResponseWriter, r *http.Request) {
	data := a.buildPageData(nil)
	renderTempl(w, r, template.AppContent(data))
}

func (a *App) handleSelectDirectory(w http.ResponseWriter, r *http.Request) {
	path, err := a.OpenDirectoryDialog()
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
	a.state.Error = ""

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleScan(w http.ResponseWriter, r *http.Request) {
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
	a.state.Error = ""
	a.logger.Info("directory scanned", "path", path, "file_count", len(files))

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handlePattern(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	a.state.Pattern = pattern
	a.state.ResetForPattern()
	a.state.PatternError = ""

	if pattern == "" {
		a.state.MatchedFiles = a.state.AllFiles
	} else {
		matched, err := a.pattern.MatchFiles(a.state.AllFiles, pattern)
		if err != nil {
			a.state.PatternError = err.Error()
			a.state.MatchedFiles = a.state.AllFiles
		} else {
			a.state.MatchedFiles = matched
		}
	}

	a.state.Error = ""
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNames(w http.ResponseWriter, r *http.Request) {
	method := r.FormValue("method")
	if method != "" {
		a.state.NamingMethod = method
	}
	action := r.FormValue("action")

	if action == "update" {
		// Collect names from form and return full content so Actions updates
		files := a.displayFiles()
		names := make([]string, len(files))
		for i := range files {
			names[i] = r.FormValue(fmt.Sprintf("name_%d", i))
		}
		a.state.NewNames = names
		a.state.ClearPreviews()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	// Method toggle only — just swap the editor panel
	renderTempl(w, r, template.NamesEditor(a.displayFiles(), a.state.NewNames, a.state.NamingMethod, a.state.Template, a.state.SearchPattern, a.state.ReplacePattern))
}

func (a *App) handleNamesGenerate(w http.ResponseWriter, r *http.Request) {
	tmpl := r.FormValue("template")
	if tmpl == "" {
		tmpl = "name_{index}"
	}
	a.state.Template = tmpl

	files := a.displayFiles()
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = domain.ExpandTemplate(tmpl, f, i)
	}
	a.state.NewNames = names
	a.state.NamingMethod = "template"
	a.state.ClearPreviews()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesFindReplace(w http.ResponseWriter, r *http.Request) {
	search := r.FormValue("search")
	replace := r.FormValue("replace")

	a.state.SearchPattern = search
	a.state.ReplacePattern = replace
	a.state.NamingMethod = "findreplace"

	files := a.displayFiles()
	names, err := domain.FindReplace(files, search, replace)
	if err != nil {
		a.state.Error = fmt.Sprintf("Invalid search pattern: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	a.state.NewNames = names
	a.state.ClearPreviews()
	a.state.Error = ""

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesUpload(w http.ResponseWriter, r *http.Request) {
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
	a.state.ClearPreviews()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleNamesLoad reads a names file by path (for drag & drop).
func (a *App) handleNamesLoad(w http.ResponseWriter, r *http.Request) {
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
	a.state.ClearPreviews()

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("clear") == "true" {
		a.state.ClearPreviews()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	files := a.displayFiles()
	previews, err := a.renamer.PreviewRename(files, a.state.NewNames)
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

	// Re-scan the directory to refresh file list
	if a.state.SelectedDirectory != "" {
		files, err := a.scanner.Scan(a.state.SelectedDirectory)
		if err == nil {
			a.state.AllFiles = files
			a.state.MatchedFiles = files
		}
	}

	renderTempl(w, r, template.MainContent(a.buildPageData(&result)))
}

func (a *App) handleUndo(w http.ResponseWriter, r *http.Request) {
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

	// Re-scan directory
	if a.state.SelectedDirectory != "" {
		files, err := a.scanner.Scan(a.state.SelectedDirectory)
		if err == nil {
			a.state.AllFiles = files
			a.state.MatchedFiles = files
		}
	}

	renderTempl(w, r, template.MainContent(a.buildPageData(&result)))
}

func (a *App) buildPageData(result interface{}) template.PageData {
	data := template.PageData{
		SelectedDirectory: a.state.SelectedDirectory,
		AllFiles:          a.state.AllFiles,
		MatchedFiles:      a.state.MatchedFiles,
		Pattern:           a.state.Pattern,
		PatternError:      a.state.PatternError,
		NewNames:          a.state.NewNames,
		Previews:          a.state.Previews,
		Error:             a.state.Error,
		NamingMethod:      a.state.NamingMethod,
		Template:          a.state.Template,
		SearchPattern:     a.state.SearchPattern,
		ReplacePattern:    a.state.ReplacePattern,
		CanUndo:           a.state.CanUndo,
	}
	if r, ok := result.(*domain.RenameResult); ok {
		data.Result = r
	}
	return data
}

func (a *App) displayFiles() []domain.FileItem {
	if a.state.Pattern != "" {
		return a.state.MatchedFiles
	}
	return a.state.AllFiles
}

func renderTempl(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = component.Render(r.Context(), w)
}
