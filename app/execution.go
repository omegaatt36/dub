package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/web/template"
)

func (a *App) handleExecute(w http.ResponseWriter, r *http.Request) {
	if countChangedPreviews(a.state.Previews) == 0 {
		a.state.Error = "No previews to execute"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	if err := validateRenameScope(a.state.Previews, a.state.SelectedDirectory); err != nil {
		a.state.Error = err.Error()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	result := a.renamer.ExecuteRename(a.state.Previews)
	if result.Success && result.RenamedCount > 0 {
		a.state.LastRenameHistory = slices.DeleteFunc(slices.Clone(a.state.Previews), func(p domain.RenamePreview) bool {
			return p.OriginalPath == p.NewPath
		})
		a.state.LastRenameDir = a.state.SelectedDirectory
		a.state.CanUndo = true
	}
	a.logger.Info("rename executed", "renamed_count", result.RenamedCount, "error_count", len(result.Errors))
	if result.Success {
		a.state.ResetForExecute()
		a.refreshFiles()
	} else {
		// A rejected operation did not consume the naming rule or manual draft.
		// Reconcile the listing and rebuild only its previews, not the user's work.
		a.state.ClearPreviews()
		a.updateRecoveryStatus()
		files, err := a.scanner.Scan(a.state.SelectedDirectory)
		if err != nil {
			a.logger.Warn("rescan after rejected rename failed", "path", a.state.SelectedDirectory, "error", err)
		} else {
			a.mergeScanned(files)
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
	if err := validateRenameScope(a.state.LastRenameHistory, a.state.LastRenameDir); err != nil {
		a.state.Error = "Undo blocked: " + err.Error()
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}

	// Preserve identity and any future safety metadata when reversing paths.
	reversed := make([]domain.RenamePreview, len(a.state.LastRenameHistory))
	for i := range a.state.LastRenameHistory {
		p := a.state.LastRenameHistory[len(a.state.LastRenameHistory)-1-i]
		p.OriginalName, p.NewName = p.NewName, p.OriginalName
		p.OriginalPath, p.NewPath = p.NewPath, p.OriginalPath
		reversed[i] = p
	}
	result := a.renamer.ExecuteRename(reversed)
	elsewhere := a.undoElsewhere()
	if result.Success {
		a.state.CanUndo = false
		a.state.LastRenameHistory = nil
		a.state.LastRenameDir = ""
		a.state.Error = ""
	}
	a.logger.Info("undo executed", "restored_count", result.RenamedCount, "error_count", len(result.Errors))
	if !elsewhere {
		if result.Success {
			a.state.ResetForExecute()
		}
		a.refreshFiles()
	}
	renderTempl(w, r, template.MainContent(a.buildPageData(&result)))
}

func validateRenameScope(previews []domain.RenamePreview, dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return fmt.Errorf("rename requires an absolute directory")
	}
	for _, p := range previews {
		if filepath.Dir(p.OriginalPath) != dir || filepath.Dir(p.NewPath) != dir ||
			filepath.Clean(p.OriginalPath) != p.OriginalPath || filepath.Clean(p.NewPath) != p.NewPath {
			return fmt.Errorf("rename paths must stay inside their original directory")
		}
	}
	return nil
}

func (a *App) undoElsewhere() bool {
	return a.state.CanUndo && a.state.LastRenameDir != "" && a.state.LastRenameDir != a.state.SelectedDirectory
}

// Recovery is explicit and ordered with execute, undo, navigation, and drops.
// Older injected renamers remain usable without implementing this optional port.
func (a *App) handleRecover(w http.ResponseWriter, r *http.Request) {
	recoverer, ok := a.renamer.(interface {
		RecoverRename(string) domain.RenameResult
	})
	if !ok || a.state.SelectedDirectory == "" {
		a.state.Error = "Recovery is not available for this directory"
		renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
		return
	}
	result := recoverer.RecoverRename(a.state.SelectedDirectory)
	// Only the locked recovery result can say that files actually changed.
	// Another process can create or remove a journal after any status check.
	if result.Success && result.RolledBack {
		a.state.ResetForExecute()
		if a.state.LastRenameDir == a.state.SelectedDirectory {
			a.state.CanUndo = false
			a.state.LastRenameHistory = nil
			a.state.LastRenameDir = ""
		}
		a.state.Error = ""
		a.refreshFiles()
	}
	a.updateRecoveryStatus()
	renderTempl(w, r, template.MainContent(a.buildPageData(&result)))
}

func (a *App) updateRecoveryStatus() {
	a.state.RecoveryPending = false
	if status, ok := a.renamer.(interface {
		RecoveryPending(string) (bool, error)
	}); ok && a.state.SelectedDirectory != "" {
		pending, err := status.RecoveryPending(a.state.SelectedDirectory)
		a.state.RecoveryPending = pending || err != nil
		if err != nil {
			a.state.Error = fmt.Sprintf("Failed to check rename recovery: %v", err)
		}
	}
}
