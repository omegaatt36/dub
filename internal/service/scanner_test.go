package service

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

// mockDirEntry implements os.DirEntry for testing.
type mockDirEntry struct {
	name  string
	isDir bool
	info  os.FileInfo
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode          { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) { return m.info, nil }

// mockFileInfo implements os.FileInfo for testing.
type mockFileInfo struct {
	name string
	size int64
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

// MockFileSystem implements port.FileSystem for testing.
type MockFileSystem struct {
	ReadDirFunc func(string) ([]os.DirEntry, error)
	StatFunc    func(string) (os.FileInfo, error)
	RenameFunc  func(string, string) error
}

func (m *MockFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return m.ReadDirFunc(path)
}

func (m *MockFileSystem) Stat(path string) (os.FileInfo, error) {
	return m.StatFunc(path)
}

func (m *MockFileSystem) Rename(oldpath, newpath string) error {
	return m.RenameFunc(oldpath, newpath)
}

func TestScannerService_Scan(t *testing.T) {
	t.Run("scans files and sorts naturally", func(t *testing.T) {
		mockFS := &MockFileSystem{
			ReadDirFunc: func(path string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockDirEntry{name: "file_10.txt", info: &mockFileInfo{name: "file_10.txt", size: 100}},
					&mockDirEntry{name: "file_2.txt", info: &mockFileInfo{name: "file_2.txt", size: 200}},
					&mockDirEntry{name: "file_1.txt", info: &mockFileInfo{name: "file_1.txt", size: 300}},
					&mockDirEntry{name: "subdir", isDir: true, info: &mockFileInfo{name: "subdir"}},
				}, nil
			},
		}

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(files) != 3 {
			t.Fatalf("got %d files, want 3 (directories excluded)", len(files))
		}

		expected := []string{"file_1.txt", "file_2.txt", "file_10.txt"}
		for i, f := range files {
			if f.Name != expected[i] {
				t.Errorf("position %d: got %q, want %q", i, f.Name, expected[i])
			}
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		mockFS := &MockFileSystem{
			ReadDirFunc: func(path string) ([]os.DirEntry, error) {
				return []os.DirEntry{}, nil
			},
		}

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/empty")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("got %d files, want 0", len(files))
		}
	})

	t.Run("extracts extension", func(t *testing.T) {
		mockFS := &MockFileSystem{
			ReadDirFunc: func(path string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockDirEntry{name: "photo.JPG", info: &mockFileInfo{name: "photo.JPG", size: 1000}},
				}, nil
			},
		}

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if files[0].Extension != ".jpg" {
			t.Errorf("got extension %q, want %q", files[0].Extension, ".jpg")
		}
	})
}
