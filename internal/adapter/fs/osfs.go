package fs

import (
	"fmt"
	"os"

	"github.com/omegaatt36/dub/internal/domain"
)

// OSFileSystem implements port.FileSystem using the real OS filesystem.
type OSFileSystem struct{}

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
	return os.Rename(oldpath, newpath)
}

func (f *OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
