package app

import (
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
	"github.com/omegaatt36/dub/web/template"
)

// GET requests render their own snapshot, without entering the operation gate.
func (a *App) handlePage(w http.ResponseWriter, r *http.Request) {
	renderTempl(w, r, template.AppContent(a.buildPageData(nil)))
}

func (a *App) handleMain(w http.ResponseWriter, r *http.Request) {
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

func (a *App) buildPageData(result *domain.RenameResult) template.PageData {
	visible := a.visibleFiles()
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
		Crumbs:            breadcrumbs(a.state.SelectedDirectory, a.home),
		Folders:           a.folderRoots(), PendingDir: a.pendingDir(),
		HideTree: a.state.HideTree, UndoDir: a.undoDir(),
		VisibleFiles: visible, ActiveFiles: a.activeFiles(),
		SelectedPaths: a.state.Selected,
		ScannedCount:  len(a.state.AllFiles), TotalCount: total, HiddenCount: hidden,
		ShowHiddenFiles: a.state.ShowHiddenFiles, SortOrder: a.state.Sort.String(),
		TickedCount:   a.state.TickCount(visible),
		ConflictCount: countConflicts(a.state.Previews),
		Pattern:       a.state.Pattern, PatternError: a.state.PatternError,
		NewNames: a.state.NewNames, Previews: a.state.Previews,
		AllowExtChange: a.state.ExtensionPolicy == port.UseProposedExtension,
		ExtChangeCount: countExtensionChanges(a.state.Previews),
		Error:          a.state.Error, NamingMethod: a.state.NamingMethod,
		Template: a.state.Template, SearchPattern: a.state.SearchPattern,
		ReplacePattern: a.state.ReplacePattern,
		CanUndo:        a.state.CanUndo, Result: result,
		RecoveryPending: a.state.RecoveryPending,
	}
}

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
