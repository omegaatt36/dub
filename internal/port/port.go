package port

import (
	"os"

	"github.com/omegaatt36/dub/internal/domain"
)

//go:generate mockgen -source=port.go -destination=../mock/mock_port.go -package=mock

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	Rename(oldpath, newpath string) error
	ReadFile(path string) ([]byte, error)
}

// ExtensionPolicy decides whether a rename may change a file's extension.
type ExtensionPolicy int

const (
	// KeepExtension preserves the original extension. Safe default.
	KeepExtension ExtensionPolicy = iota
	// UseProposedExtension lets a template set the extension explicitly.
	UseProposedExtension
)

// PatternMatcher abstracts pattern matching for testability.
type PatternMatcher interface {
	ExpandShortcuts(pattern string) string
	// Compile validates the pattern once and returns a matcher reusable for all
	// filenames in a scan. The returned function must be safe for concurrent use.
	Compile(pattern string) (func(string) bool, error)
}

// Scanner scans directories for files and folders.
type Scanner interface {
	Scan(path string) ([]domain.FileItem, error)
	// ListFolders returns the immediate subdirectories of path.
	ListFolders(path string) ([]domain.FolderItem, error)
}

// PatternFilter filters files by pattern.
type PatternFilter interface {
	MatchFiles(files []domain.FileItem, pattern string) ([]domain.FileItem, error)
}

// Renamer handles rename previewing and execution.
type Renamer interface {
	PreviewRename(files []domain.FileItem, newNames []string, policy ExtensionPolicy) ([]domain.RenamePreview, error)
	ExecuteRename(previews []domain.RenamePreview) domain.RenameResult
}

type RenameDirectory interface {
	Identity() (string, error)
	// EntryIdentity uses directory-relative, no-follow stat.
	EntryIdentity(name string) (string, error)
	// CaseSensitive queries native directory capabilities without probe writes.
	// Unknown must use conservative Unicode folding, not infer from entry aliases.
	CaseSensitive() (sensitive, known bool)
	// Lock serializes execution and recovery, including across processes.
	Lock() error
	ReadJournal() ([]byte, error)
	CreateJournal(data []byte) error
	RemoveJournal() error
	ReadDir() ([]os.DirEntry, error)
	Rename(oldName, newName string) error
	Close() error
}

type RenameDirectoryOpener interface {
	OpenRenameDirectory(path string) (RenameDirectory, error)
}
