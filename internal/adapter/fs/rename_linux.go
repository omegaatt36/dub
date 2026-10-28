package fs

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func entryIdentity(fd int, name string) (string, error) {
	return statIdentity(fd, name, unix.AT_SYMLINK_NOFOLLOW)
}

func fileIdentity(fd int) (string, error) {
	return statIdentity(fd, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW)
}

func statIdentity(fd int, name string, flags int) (string, error) {
	var stat unix.Statx_t
	if err := unix.Statx(fd, name, flags, unix.STATX_INO|unix.STATX_TYPE|unix.STATX_BTIME, &stat); err != nil {
		return "", err
	}
	if stat.Mask&unix.STATX_BTIME == 0 {
		return "", fmt.Errorf("filesystem does not provide stable entry birth identity")
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d", stat.Dev_major, stat.Dev_minor, stat.Ino, stat.Mode&unix.S_IFMT, stat.Btime.Sec, stat.Btime.Nsec), nil
}

func directoryCaseSensitive(fd int) (bool, bool) {
	var stat unix.Statfs_t
	if err := unix.Fstatfs(fd, &stat); err != nil {
		return false, false
	}
	// Only these local filesystems have a known per-directory casefold flag.
	// Absence of that flag on arbitrary network/FUSE filesystems proves nothing.
	if stat.Type != unix.EXT4_SUPER_MAGIC && stat.Type != unix.F2FS_SUPER_MAGIC {
		return false, false
	}
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return false, false
	}
	const fsCasefold = 0x40000000
	return flags&fsCasefold == 0, true
}

func renameExclusive(oldFD int, oldName string, newFD int, newName string) error {
	return unix.Renameat2(oldFD, oldName, newFD, newName, unix.RENAME_NOREPLACE)
}
