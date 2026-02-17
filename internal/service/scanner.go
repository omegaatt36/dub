package service

import (
	"path/filepath"
	"strings"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// ScannerService scans directories for files.
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
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		files = append(files, domain.FileItem{
			Name:      name,
			Path:      filepath.Join(path, name),
			Extension: ext,
			Size:      uint64(info.Size()),
			ModTime:   info.ModTime(),
		})
	}

	domain.NaturalSort(files)
	return files, nil
}
