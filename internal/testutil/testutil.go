package testutil

import (
	"io/fs"
	"os"
	"time"
)

// MockDirEntry implements os.DirEntry for testing.
type MockDirEntry struct {
	EntryName string
	Dir       bool
	FileInfo  os.FileInfo
}

func (m *MockDirEntry) Name() string               { return m.EntryName }
func (m *MockDirEntry) IsDir() bool                { return m.Dir }
func (m *MockDirEntry) Type() fs.FileMode          { return 0 }
func (m *MockDirEntry) Info() (os.FileInfo, error) { return m.FileInfo, nil }

// MockFileInfo implements os.FileInfo for testing.
type MockFileInfo struct {
	FileName string
	FileSize int64
}

func (m *MockFileInfo) Name() string       { return m.FileName }
func (m *MockFileInfo) Size() int64        { return m.FileSize }
func (m *MockFileInfo) Mode() fs.FileMode  { return 0o644 }
func (m *MockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *MockFileInfo) IsDir() bool        { return false }
func (m *MockFileInfo) Sys() any           { return nil }

// NewMockDirEntry creates a MockDirEntry for a file with the given name and size.
func NewMockDirEntry(name string, size int64) *MockDirEntry {
	return &MockDirEntry{
		EntryName: name,
		FileInfo:  &MockFileInfo{FileName: name, FileSize: size},
	}
}

// NewMockDirDirEntry creates a MockDirEntry for a directory.
func NewMockDirDirEntry(name string) *MockDirEntry {
	return &MockDirEntry{
		EntryName: name,
		Dir:       true,
		FileInfo:  &MockFileInfo{FileName: name},
	}
}
