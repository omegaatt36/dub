//go:build darwin || linux

package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/omegaatt36/dub/internal/port"
)

type renameDirectory struct {
	file          *os.File
	journalID     string
	journalOpened func() // Session-local test seam after open, before stat.
}

const journalName = ".dub-rename-journal"

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
	if d.journalID != "" {
		if err := d.verifyJournal(); err != nil {
			return err
		}
	}
	for _, n := range []string{oldName, newName} {
		if n == "" || n == "." || n == ".." || strings.ContainsAny(n, "/\\\x00") {
			return fmt.Errorf("invalid filename %q", n)
		}
	}
	return renameExclusive(int(d.file.Fd()), oldName, int(d.file.Fd()), newName)
}

func (d *renameDirectory) Identity() (string, error) {
	return entryIdentity(int(d.file.Fd()), ".")
}

func validEntry(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

func (d *renameDirectory) EntryIdentity(name string) (string, error) {
	if !validEntry(name) {
		return "", fmt.Errorf("invalid entry name")
	}
	return entryIdentity(int(d.file.Fd()), name)
}

func (d *renameDirectory) Lock() error {
	return unix.Flock(int(d.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func (d *renameDirectory) ReadJournal() ([]byte, error) {
	fd, err := unix.Openat(int(d.file.Fd()), journalName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), journalName)
	defer f.Close()
	if err := d.bindJournal(fd); err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return nil, fmt.Errorf("invalid rename journal")
	}
	data, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, fmt.Errorf("rename journal exceeds size limit")
	}
	if err := d.verifyJournal(); err != nil {
		return nil, err
	}
	return data, nil
}

func (d *renameDirectory) CreateJournal(data []byte) error {
	fd, err := unix.Openat(int(d.file.Fd()), journalName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), journalName)
	defer f.Close()
	if err := d.bindJournal(fd); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := d.verifyJournal(); err != nil {
		return err
	}
	return d.file.Sync()
}

func (d *renameDirectory) bindJournal(fd int) error {
	if d.journalOpened != nil {
		d.journalOpened()
	}
	id, err := fileIdentity(fd)
	if err != nil {
		return err
	}
	if d.journalID != "" && d.journalID != id {
		return fmt.Errorf("rename journal was replaced")
	}
	d.journalID = id
	return d.verifyJournal()
}

func (d *renameDirectory) verifyJournal() error {
	id, err := d.EntryIdentity(journalName)
	if err != nil {
		return err
	}
	if d.journalID == "" || d.journalID != id {
		return fmt.Errorf("rename journal was replaced")
	}
	return nil
}

func (d *renameDirectory) RemoveJournal() error {
	id, err := d.EntryIdentity(journalName)
	if err != nil {
		return err
	}
	if d.journalID == "" || d.journalID != id {
		return fmt.Errorf("rename journal was replaced; refusing cleanup")
	}
	if err := unix.Unlinkat(int(d.file.Fd()), journalName, 0); err != nil {
		return err
	}
	return d.file.Sync()
}

// CaseSensitive queries native capabilities without writing probe entries.
func (d *renameDirectory) CaseSensitive() (bool, bool) {
	return directoryCaseSensitive(int(d.file.Fd()))
}
