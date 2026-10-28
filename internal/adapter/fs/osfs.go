package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/omegaatt36/dub/internal/domain"
)

// OSFileSystem implements port.FileSystem using the real OS filesystem.
type OSFileSystem struct{}

// OpenFile lets import callers enforce a read limit before allocating the file.
func (f *OSFileSystem) OpenFile(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (f *OSFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidPath, err)
	}
	return entries, nil
}

func (f *OSFileSystem) Stat(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidPath, err)
	}
	return info, nil
}

func (f *OSFileSystem) Rename(oldpath, newpath string) error {
	if filepath.Dir(oldpath) != filepath.Dir(newpath) {
		return fmt.Errorf("rename must stay in one directory")
	}
	session, err := f.OpenRenameDirectory(filepath.Dir(oldpath))
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Rename(filepath.Base(oldpath), filepath.Base(newpath))
}

func (f *OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
