package fs

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func entryIdentity(fd int, name string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Mode&unix.S_IFMT, stat.Btim.Sec, stat.Btim.Nsec), nil
}

func fileIdentity(fd int) (string, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Mode&unix.S_IFMT, stat.Btim.Sec, stat.Btim.Nsec), nil
}

func directoryCaseSensitive(fd int) (bool, bool) {
	// Darwin <unistd.h>: _PC_CASE_SENSITIVE. x/sys does not export it.
	const pcCaseSensitive = 11
	value, err := unix.Fpathconf(fd, pcCaseSensitive)
	return value == 1, err == nil && (value == 0 || value == 1)
}

func renameExclusive(oldFD int, oldName string, newFD int, newName string) error {
	return unix.RenameatxNp(oldFD, oldName, newFD, newName, unix.RENAME_EXCL)
}
