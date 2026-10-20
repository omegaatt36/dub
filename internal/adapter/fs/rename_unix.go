//go:build darwin || linux

package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/omegaatt36/dub/internal/port"
)

type renameDirectory struct{ file *os.File }

func (f *OSFileSystem) OpenRenameDirectory(path string) (port.RenameDirectory, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("directory must be canonical and absolute")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open rename directory: %w", err)
	}
	return &renameDirectory{file: os.NewFile(uintptr(fd), path)}, nil
}
func (d *renameDirectory) ReadDir() ([]os.DirEntry, error) { return d.file.ReadDir(-1) }
func (d *renameDirectory) Close() error                    { return d.file.Close() }
func (d *renameDirectory) Rename(oldName, newName string) error {
	for _, n := range []string{oldName, newName} {
		if n == "" || n == "." || n == ".." || strings.ContainsAny(n, "/\\\x00") {
			return fmt.Errorf("invalid filename %q", n)
		}
	}
	return renameExclusive(int(d.file.Fd()), oldName, int(d.file.Fd()), newName)
}

func (d *renameDirectory) Identity() (string, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(d.file.Fd()), &stat); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
