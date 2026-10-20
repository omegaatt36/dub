package app

import (
	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// AppState holds the entire application state. All state is server-side.
type AppState struct {
	SelectedDirectory string
	AllFiles          []domain.FileItem
	// MatchedFiles is what the file list shows: AllFiles after the hidden-file
	// rule and the pattern. applyFilter is its only writer, so every handler
	// that changes either input must call it.
	MatchedFiles []domain.FileItem
	// Selected holds the paths the user has ticked in the file list. It is the
	// only set that may be renamed; an empty set means "nothing is ticked",
	// never "everything is ticked". Scanning selects every file so the common
	// case (rename all, then tick some off) needs no extra clicks.
	Selected        map[string]bool
	ShowHiddenFiles bool // Session preference: include dotfiles in the list and rename set.
	// Sort orders AllFiles and therefore {index}. Like ShowHiddenFiles it is a
	// session preference and survives a directory change.
	Sort domain.SortOrder
	// ManualNames remembers what the user typed per file, so unticking a file
	// and ticking it again does not lose its name. NewNames is positional and
	// cannot do that.
	ManualNames    map[string]string
	Pattern        string
	PatternError   string
	NewNames       []string
	Previews       []domain.RenamePreview
	Error          string
	NamingMethod   string // "manual" | "file" | "template" | "findreplace"
	AppliedMethod  string
	LoadedNames    []string
	Template       string
	SearchPattern  string
	ReplacePattern string
	// ExtensionPolicy decides whether a rename may change a file's extension.
	// It defaults to port.KeepExtension; the UI opts in explicitly.
	ExtensionPolicy port.ExtensionPolicy
	// LastRenameHistory is what Undo reverses. It outlives a directory change on
	// purpose: its paths are absolute, so undo works from anywhere, and with the
	// folder tree one stray click would otherwise cost the user the undo.
	// LastRenameDir is where that rename happened, so the UI can say so.
	LastRenameHistory []domain.RenamePreview
	LastRenameDir     string
	CanUndo           bool
	// NamesTyped records that the current names came from the manual editor. The
	// names themselves cannot say so, and only typed names are lost for good on
	// a directory change: a template or search rule can be re-applied, a loaded
	// file can be loaded again.
	NamesTyped bool
	// HideTree collapses the folder sidebar to give the file panes the width.
	HideTree bool
	// Expanded is the set of folder-tree nodes that are open. Navigation adds
	// the chain down to the new directory, and the user can still collapse any
	// of it. Like the sort order it belongs to the session, not to a directory,
	// so ResetForDirectory leaves it alone.
	Expanded map[string]bool
	// ExtraRoots are the tree's roots beyond the home folder, one per place the
	// user opened outside it (a USB disk, /tmp). A root stays put while the user
	// navigates inside it, so the tree never loses the path back up.
	ExtraRoots []string
	// PendingDir is a directory the user asked for while typed names were at
	// stake. It waits here until they discard the names or keep editing.
	PendingDir string
}

func NewAppState() *AppState {
	return &AppState{
		NamingMethod: "manual",
		Template:     "name_{index}",
		Selected:     make(map[string]bool),
		ManualNames:  make(map[string]string),
		Expanded:     make(map[string]bool),
		Sort:         domain.SortOrder{Field: domain.SortByName},
	}
}

// SelectAllTicks marks every file in the given list as ticked and leaves other
// ticks alone, so ticking what is on screen never un-ticks what is not.
func (s *AppState) SelectAllTicks(files []domain.FileItem) {
	for _, f := range files {
		s.Selected[f.Path] = true
	}
}

// PruneTicks drops ticks for files that are no longer in the scanned list, so
// the set cannot grow without bound across renames.
func (s *AppState) PruneTicks(scanned []domain.FileItem) {
	live := make(map[string]bool, len(scanned))
	for _, f := range scanned {
		live[f.Path] = true
	}
	for path := range s.Selected {
		if !live[path] {
			delete(s.Selected, path)
		}
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
// Invariant: no file list, filter, names, previews or ticks from the old
// directory may survive, because they describe files that are gone. The undo
// history is the exception: it is keyed by absolute path and stays valid.
func (s *AppState) ResetForDirectory() {
	s.AllFiles = nil
	s.MatchedFiles = nil
	clear(s.Selected)
	clear(s.ManualNames)
	s.Pattern = ""
	s.PatternError = ""
	s.NewNames = nil
	s.Previews = nil
	s.Error = ""
	s.NamesTyped = false
	s.AppliedMethod = ""
	s.LoadedNames = nil
	s.ExtensionPolicy = port.KeepExtension
}

// ClearPreviews drops the preview set without touching names or the filter.
// Invariant: the files and their proposed names are unchanged, so the user can
// regenerate previews without retyping anything.
func (s *AppState) ClearPreviews() {
	s.Previews = nil
}

// ResetForExecute clears the one-shot rename inputs after execution.
// Invariant: previews and names are consumed by the rename and must not be
// resubmitted, and the per-file names are keyed by paths the rename just
// changed. Pattern and MatchedFiles survive so the user can rename the next
// batch from the same filtered view; callers must re-apply the filter to
// freshly scanned files.
func (s *AppState) ResetForExecute() {
	s.NewNames = nil
	s.Previews = nil
	s.NamesTyped = false
	s.AppliedMethod = ""
	s.LoadedNames = nil
	clear(s.ManualNames)
}
