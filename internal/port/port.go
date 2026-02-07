package port

import "os"

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	Rename(oldpath, newpath string) error
}

// PatternMatcher abstracts pattern matching for testability.
type PatternMatcher interface {
	ExpandShortcuts(pattern string) string
	Match(pattern, name string) (bool, error)
}
