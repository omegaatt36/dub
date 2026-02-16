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

// PatternMatcher abstracts pattern matching for testability.
type PatternMatcher interface {
	ExpandShortcuts(pattern string) string
	Match(pattern, name string) (bool, error)
}

// Scanner scans directories for files.
type Scanner interface {
	Scan(path string) ([]domain.FileItem, error)
}

// PatternFilter filters files by pattern.
type PatternFilter interface {
	MatchFiles(files []domain.FileItem, pattern string) ([]domain.FileItem, error)
}

// Renamer handles rename previewing and execution.
type Renamer interface {
	PreviewRename(files []domain.FileItem, newNames []string) ([]domain.RenamePreview, error)
	ExecuteRename(previews []domain.RenamePreview) domain.RenameResult
}
