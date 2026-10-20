package fs

import "golang.org/x/sys/unix"

func renameExclusive(oldFD int, oldName string, newFD int, newName string) error {
	return unix.Renameat2(oldFD, oldName, newFD, newName, unix.RENAME_NOREPLACE)
}
