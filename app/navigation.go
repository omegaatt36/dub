package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/web/template"
)

func (a *App) handleSelectDirectory(w http.ResponseWriter, r *http.Request) {
	// Prompt outside the operation gate so other UI actions and views remain
	// responsive while the native OS dialog is open.
	path, err := a.OpenDirectoryDialog()

	if err != nil || path == "" {
		// User cancelled the dialog or error — return current state unchanged
		work := a.snapshot()
		work.readOnly = true
		renderTempl(w, r, template.MainContent(work.buildPageData(nil)))
		return
	}

	a.operations.Lock()
	defer a.operations.Unlock()
	work := a.snapshot()
	work.requestScan(path)
	renderTempl(w, r, template.MainContent(work.buildPageData(nil)))
	a.publish(work)
}

func (a *App) handleScan(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	switch {
	case path == "":
		a.state.Error = "No directory path provided"
	case r.FormValue("discard") == "true":
		a.scanPath(path)
	default:
		a.requestScan(path)
	}

	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// resolveDir narrows a file path to its parent directory, because a desktop
// drop can hand over either a folder or a single file.
func (a *App) resolveDir(path string) string {
	if info, err := a.fs.Stat(path); err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}

// scanPath points the app at path and loads its files, dropping everything tied
// to the previous directory. The scan comes first: a folder that cannot be read
// (a locked one, one that just vanished) leaves the user exactly where they
// were, with the reason, instead of on an empty page that has lost their work.
// Called only on a private operation snapshot.
func (a *App) scanPath(path string) {
	path = a.resolveDir(path)
	a.state.PendingDir = ""

	files, err := a.scanner.Scan(path)
	if err != nil {
		a.state.Error = fmt.Sprintf("Failed to scan directory: %v", err)
		return
	}

	a.state.SelectedDirectory = path
	a.revealInTree(path)
	a.state.ResetForDirectory()
	a.setScanned(files)
	a.state.Error = ""
	a.updateRecoveryStatus()
	a.logger.Info("directory scanned", "path", path, "file_count", len(files))
}

// handleTick toggles one file's checkbox.
func (a *App) handleTick(w http.ResponseWriter, r *http.Request) {
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
	pattern := r.FormValue("pattern")
	a.state.Error = ""
	a.changeFileSet(func() {
		a.state.Pattern = pattern
		a.applyFilter()
	})
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleRefresh rescans the folder in view for changes made outside the app.
// The user's ticks and naming work carry over; only files that are new start
// ticked, as after any scan. The silent form is the background check the page
// sends when the window regains focus: it answers 204, which htmx leaves
// unswapped, unless the listing really changed, and it never raises an error
// the user did not ask for.
func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	silent := r.FormValue("silent") == "true"
	dir := a.state.SelectedDirectory
	if dir == "" {
		a.answerRefresh(w, r, silent, false)
		return
	}

	files, err := a.scanner.Scan(dir)
	if err != nil {
		if !silent {
			a.state.Error = fmt.Sprintf("Failed to refresh directory: %v", err)
		}
		a.answerRefresh(w, r, silent, false)
		return
	}

	previousRecovery := a.state.RecoveryPending
	a.updateRecoveryStatus()
	changed := !sameListing(a.state.AllFiles, files)
	if !silent {
		a.folderCache = nil
		if !a.state.RecoveryPending {
			a.state.Error = ""
		}
	}
	if changed {
		a.mergeScanned(files)
	}
	a.answerRefresh(w, r, silent, changed || previousRecovery != a.state.RecoveryPending || !silent)
}

// answerRefresh renders the page, or nothing when a silent refresh found
// nothing to show.
func (a *App) answerRefresh(w http.ResponseWriter, r *http.Request, silent, render bool) {
	if silent && !render {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// sameListing reports whether two scans of one folder describe identical files.
// Order is ignored: the held list is sorted the way the user asked, a fresh scan
// is not, and the same files in a different order are still no change.
func sameListing(a, b []domain.FileItem) bool {
	if len(a) != len(b) {
		return false
	}
	held := make(map[string]domain.FileItem, len(a))
	for _, f := range a {
		held[f.Path] = f
	}
	for _, f := range b {
		old, ok := held[f.Path]
		if !ok || old.Size != f.Size || !old.ModTime.Equal(f.ModTime) {
			return false
		}
	}
	return true
}

// mergeScanned installs a rescan of the same folder. Unlike setScanned it ticks
// only the files it has not seen before, so a refresh never undoes the user's
// choice to leave a file out.
func (a *App) mergeScanned(files []domain.FileItem) {
	known := make(map[string]bool, len(a.state.AllFiles))
	for _, f := range a.state.AllFiles {
		known[f.Path] = true
	}

	a.changeFileSet(func() {
		domain.SortFiles(files, a.state.Sort)
		a.state.AllFiles = files
		a.state.PruneTicks(files)
		a.applyFilter()
		for _, f := range a.state.MatchedFiles {
			if !known[f.Path] {
				a.state.Selected[f.Path] = true
			}
		}
	})
}

// applyFilter recomputes MatchedFiles from the hidden-file rule and the current
// Pattern. It is the one place visibility is decided; everything else reads
// MatchedFiles. An invalid pattern is not fatal: the view falls back to the
// unfiltered list and reports the reason through PatternError.
// Called only on a private operation snapshot.
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
// rule stay unticked until the user ticks them.
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
// Called only on a private operation snapshot.
func (a *App) changeFileSet(mutate func()) {
	applied := a.state.AppliedMethod != ""
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
	if a.state.AppliedMethod != "manual" {
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
// Called only on a private operation snapshot.
func (a *App) rebuildNames() {
	files := a.activeFiles()
	if len(files) == 0 {
		a.state.NewNames = nil
		a.state.Previews = nil
		return
	}

	switch a.state.AppliedMethod {
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
	case "file":
		a.state.NewNames = slices.Clone(a.state.LoadedNames)
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
// Called only on a private operation snapshot.
func (a *App) refreshFiles() {
	if a.state.SelectedDirectory == "" {
		return
	}

	a.updateRecoveryStatus()
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
