package app

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
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
	mux.HandleFunc("POST /api/names/upload", a.handleNamesUpload)
	mux.HandleFunc("POST /api/preview", a.handlePreview)
	mux.HandleFunc("POST /api/execute", a.handleExecute)

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

func (a *App) handlePattern(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	a.state.Pattern = pattern
	a.state.ResetForPattern()

	if pattern == "" {
		a.state.MatchedFiles = a.state.AllFiles
	} else {
		matched, err := a.pattern.MatchFiles(a.state.AllFiles, pattern)
		if err != nil {
			a.state.Error = fmt.Sprintf("Invalid pattern: %v", err)
			a.state.MatchedFiles = a.state.AllFiles
		} else {
			a.state.MatchedFiles = matched
			a.state.Error = ""
		}
	}

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
	renderTempl(w, r, template.NamesEditor(a.displayFiles(), a.state.NewNames, a.state.NamingMethod, a.state.Template))
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
		name := tmpl
		name = strings.ReplaceAll(name, "{index}", fmt.Sprintf("%d", i+1))
		origName := strings.TrimSuffix(f.Name, f.Extension)
		name = strings.ReplaceAll(name, "{original}", origName)
		names[i] = name
	}
	a.state.NewNames = names
	a.state.NamingMethod = "template"
	a.state.ClearPreviews()

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

func (a *App) buildPageData(result interface{}) template.PageData {
	data := template.PageData{
		SelectedDirectory: a.state.SelectedDirectory,
		AllFiles:          a.state.AllFiles,
		MatchedFiles:      a.state.MatchedFiles,
		Pattern:           a.state.Pattern,
		NewNames:          a.state.NewNames,
		Previews:          a.state.Previews,
		Error:             a.state.Error,
		NamingMethod:      a.state.NamingMethod,
		Template:          a.state.Template,
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
