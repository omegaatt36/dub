package app

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
	"github.com/omegaatt36/dub/web/template"
)

func (a *App) handleExtensionPolicy(w http.ResponseWriter, r *http.Request) {
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
	if method := r.FormValue("method"); method != "" {
		a.state.NamingMethod = method
	}
	if r.FormValue("action") == "update" {
		files := a.activeFiles()
		names := make([]string, len(files))
		for i := range files {
			names[i] = r.FormValue(fmt.Sprintf("name_%d", i))
		}
		a.state.NewNames = names
		a.state.NamesTyped = true
		a.state.AppliedMethod = "manual"
		a.rememberManualNames()
		a.autoPreview()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	renderTempl(w, r, template.NamesEditor(template.EditorData{
		Files: a.activeFiles(), Names: a.state.NewNames,
		Method: a.state.NamingMethod, Template: a.state.Template,
		Search: a.state.SearchPattern, Replace: a.state.ReplacePattern,
		AllowExtChange: a.state.ExtensionPolicy == port.UseProposedExtension,
	}))
}

func (a *App) handleNamesGenerate(w http.ResponseWriter, r *http.Request) {
	tmpl := r.FormValue("template")
	if tmpl == "" {
		tmpl = "name_{index}"
	}
	a.state.Template = tmpl
	a.state.NewNames = templateNames(a.activeFiles(), tmpl)
	a.state.NamesTyped = false
	a.state.NamingMethod = "template"
	a.state.AppliedMethod = "template"
	a.state.Error = ""
	a.autoPreview()
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesFindReplace(w http.ResponseWriter, r *http.Request) {
	search, replace := r.FormValue("search"), r.FormValue("replace")
	a.state.SearchPattern = search
	a.state.ReplacePattern = replace
	a.state.NamingMethod = "findreplace"
	names, err := domain.FindReplace(a.activeFiles(), search, replace)
	if err != nil {
		a.state.Error = fmt.Sprintf("Invalid search pattern: %v", err)
		a.state.Previews = nil
		a.state.NewNames = nil
		a.state.AppliedMethod = ""
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	a.state.NewNames = names
	a.state.NamesTyped = false
	a.state.AppliedMethod = "findreplace"
	a.state.Error = ""
	a.autoPreview()
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesUpload(w http.ResponseWriter, r *http.Request) {
	// Bound both the multipart envelope and the names stream, and remove
	// temporary multipart files on every exit, including parse failures.
	r.Body = http.MaxBytesReader(w, r.Body, maxNamesBytes+(64<<10))
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if err := r.ParseMultipartForm(64 << 10); err != nil {
		a.state.Error = fmt.Sprintf("Failed to read uploaded file: %v", err)
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	file, _, err := r.FormFile("namesfile")
	if err != nil {
		a.state.Error = "Failed to read uploaded file"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	defer file.Close()
	names, err := namesFromLines(file)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to read names: %v", err)
	} else {
		a.installLoadedNames(names)
	}
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) handleNamesLoad(w http.ResponseWriter, r *http.Request) {
	if path := r.FormValue("path"); path == "" {
		a.state.Error = "No file path provided"
	} else {
		a.loadNamesPath(path)
	}
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) loadNamesPath(path string) {
	names, err := a.readNamesPath(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to read file: %v", err)
		return
	}
	a.installLoadedNames(names)
}

func (a *App) installLoadedNames(names []string) {
	a.state.NewNames = names
	a.state.NamesTyped = false
	a.state.NamingMethod = "file"
	a.state.AppliedMethod = "file"
	a.state.LoadedNames = slices.Clone(names)
	a.state.Error = ""
	a.autoPreview()
}

func (a *App) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("clear") == "true" {
		a.state.ResetForExecute()
		a.state.Error = ""
	} else {
		previews, err := a.renamer.PreviewRename(a.activeFiles(), a.state.NewNames, a.state.ExtensionPolicy)
		if err != nil {
			a.state.Error = fmt.Sprintf("Preview failed: %v", err)
		} else {
			a.state.Previews = previews
			a.state.Error = ""
		}
	}
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}
