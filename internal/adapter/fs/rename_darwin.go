package fs

import "golang.org/x/sys/unix"

func renameExclusive(oldFD int, oldName string, newFD int, newName string) error {
	return unix.RenameatxNp(oldFD, oldName, newFD, newName, unix.RENAME_EXCL)
}
