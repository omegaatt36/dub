package app

import (
	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// AppState holds the entire application state. All state is server-side.
type AppState struct {
	SelectedDirectory string
	AllFiles          []domain.FileItem
	MatchedFiles      []domain.FileItem
	// Selected holds the paths the user has ticked in the file list. It is the
	// only set that may be renamed; an empty set means "nothing is ticked",
	// never "everything is ticked". Scanning selects every file so the common
	// case (rename all, then tick some off) needs no extra clicks.
	Selected       map[string]bool
	Pattern        string
	PatternError   string
	NewNames       []string
	Previews       []domain.RenamePreview
	Error          string
	NamingMethod   string // "manual" | "file" | "template" | "findreplace"
	Template       string
	SearchPattern  string
	ReplacePattern string
	// ExtensionPolicy decides whether a rename may change a file's extension.
	// It defaults to port.KeepExtension; the UI opts in explicitly.
	ExtensionPolicy   port.ExtensionPolicy
	LastRenameHistory []domain.RenamePreview
	CanUndo           bool
}

func NewAppState() *AppState {
	return &AppState{
		NamingMethod: "manual",
		Template:     "name_{index}",
		Selected:     make(map[string]bool),
	}
}

// SelectAllTicks marks every file in the given list as ticked. Files that
// vanished from disk are pruned so the set cannot grow without bound.
func (s *AppState) SelectAllTicks(files []domain.FileItem) {
	live := make(map[string]bool, len(files))
	for _, f := range files {
		live[f.Path] = true
	}
	for path := range s.Selected {
		if !live[path] {
			delete(s.Selected, path)
		}
	}
	for _, f := range files {
		s.Selected[f.Path] = true
	}
}

// TickCount reports how many of the given files are currently ticked.
func (s *AppState) TickCount(files []domain.FileItem) int {
	n := 0
	for _, f := range files {
		if s.Selected[f.Path] {
			n++
		}
	}
	return n
}

// ResetForDirectory clears everything tied to the previous directory.
// Invariant: no file list, filter, names, previews, ticks or undo history from
// the old directory may survive, because they describe files that are gone.
func (s *AppState) ResetForDirectory() {
	s.AllFiles = nil
	s.MatchedFiles = nil
	clear(s.Selected)
	s.Pattern = ""
	s.PatternError = ""
	s.NewNames = nil
	s.Previews = nil
	s.Error = ""
	s.CanUndo = false
	s.LastRenameHistory = nil
	s.ExtensionPolicy = port.KeepExtension
}

// ResetForPattern clears the match-dependent state when the filter changes.
// Invariant: the caller is about to recompute MatchedFiles from Pattern, and
// any names/previews built from the previous match are no longer trustworthy.
// Ticks are deliberately NOT cleared: narrowing a filter should not silently
// un-tick the files the user already chose. PatternError is cleared here so it
// is never stale; the caller sets it only if the new pattern fails to compile.
func (s *AppState) ResetForPattern() {
	s.MatchedFiles = nil
	s.NewNames = nil
	s.Previews = nil
	s.PatternError = ""
}

// ClearPreviews drops the preview set without touching names or the filter.
// Invariant: the files and their proposed names are unchanged, so the user can
// regenerate previews without retyping anything.
func (s *AppState) ClearPreviews() {
	s.Previews = nil
}

// ResetForExecute clears the one-shot rename inputs after execution.
// Invariant: previews and names are consumed by the rename and must not be
// resubmitted, but Pattern and MatchedFiles survive so the user can rename the
// next batch from the same filtered view. Callers must re-apply the filter to
// freshly scanned files.
func (s *AppState) ResetForExecute() {
	s.NewNames = nil
	s.Previews = nil
}
