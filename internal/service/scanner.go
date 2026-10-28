package service

import (
	"os"
	"path/filepath"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// ScannerService scans directories for files and folders.
type ScannerService struct {
	fs port.FileSystem
}

func NewScannerService(fs port.FileSystem) *ScannerService {
	return &ScannerService{fs: fs}
}

// Scan reads a directory and returns sorted file items (files only, no directories).
func (s *ScannerService) Scan(path string) ([]domain.FileItem, error) {
	entries, err := s.fs.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var files []domain.FileItem
	for _, entry := range entries {
		if domain.IsRenameInternal(entry.Name()) || s.isDir(path, entry) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := entry.Name()
		files = append(files, domain.FileItem{
			Name:      name,
			Path:      filepath.Join(path, name),
			Extension: domain.Extension(name),
			Size:      uint64(info.Size()),
			ModTime:   info.ModTime(),
		})
	}

	domain.NaturalSort(files)
	return files, nil
}

// ListFolders returns the immediate subdirectories of path in natural name
// order. It reads one level only, so a folder tree can load lazily.
func (s *ScannerService) ListFolders(path string) ([]domain.FolderItem, error) {
	entries, err := s.fs.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var folders []domain.FolderItem
	for _, entry := range entries {
		if domain.IsRenameInternal(entry.Name()) || !s.isDir(path, entry) {
			continue
		}

		name := entry.Name()
		folders = append(folders, domain.FolderItem{
			Name: name,
			Path: filepath.Join(path, name),
		})
	}

	domain.SortFolders(folders)
	return folders, nil
}

// isDir reports whether entry is a folder, looking through a symlink. Links to
// folders are common (a Downloads folder moved to another disk, a synced
// folder), so they belong in the folder tree rather than in the file list.
// Browsing never recurses on its own, so a link cycle cannot loop.
func (s *ScannerService) isDir(dir string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := s.fs.Stat(filepath.Join(dir, entry.Name()))
	return err == nil && info.IsDir()
}
