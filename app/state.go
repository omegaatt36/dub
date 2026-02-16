package app

import "github.com/omegaatt36/dub/internal/domain"

// AppState holds the entire application state. All state is server-side.
type AppState struct {
	SelectedDirectory string
	AllFiles          []domain.FileItem
	MatchedFiles      []domain.FileItem
	Pattern           string
	NewNames          []string
	Previews          []domain.RenamePreview
	IsLoading         bool
	Error             string
	NamingMethod      string // "manual" | "file" | "template"
	Template          string
	LastRenameHistory []domain.RenamePreview
	CanUndo           bool
}

func NewAppState() *AppState {
	return &AppState{
		NamingMethod: "manual",
		Template:     "name_{index}",
	}
}

// ResetForDirectory clears state when a new directory is selected.
func (s *AppState) ResetForDirectory() {
	s.AllFiles = nil
	s.MatchedFiles = nil
	s.Pattern = ""
	s.NewNames = nil
	s.Previews = nil
	s.Error = ""
	s.CanUndo = false
	s.LastRenameHistory = nil
}

// ResetForPattern clears match-dependent state when pattern changes.
func (s *AppState) ResetForPattern() {
	s.MatchedFiles = nil
	s.NewNames = nil
	s.Previews = nil
}

// ClearPreviews removes preview data.
func (s *AppState) ClearPreviews() {
	s.Previews = nil
}

// ResetForExecute resets state after a successful rename execution.
func (s *AppState) ResetForExecute() {
	s.NewNames = nil
	s.Previews = nil
	s.Pattern = ""
}
