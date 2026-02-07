package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// RenamerService handles rename previewing and execution.
type RenamerService struct {
	fs port.FileSystem
}

func NewRenamerService(fs port.FileSystem) *RenamerService {
	return &RenamerService{fs: fs}
}

// PreviewRename generates rename previews from matched files and new names.
// It appends the original file extension to each new name and detects conflicts.
func (s *RenamerService) PreviewRename(files []domain.FileItem, newNames []string) ([]domain.RenamePreview, error) {
	if len(files) != len(newNames) {
		return nil, domain.ErrMismatchedNames
	}

	previews := make([]domain.RenamePreview, len(files))
	for i, f := range files {
		newName := strings.TrimSpace(newNames[i])
		if newName == "" {
			newName = f.Name
		} else if f.Extension != "" && !strings.HasSuffix(strings.ToLower(newName), strings.ToLower(f.Extension)) {
			newName = newName + f.Extension
		}

		dir := filepath.Dir(f.Path)
		previews[i] = domain.RenamePreview{
			OriginalName: f.Name,
			NewName:      newName,
			OriginalPath: f.Path,
			NewPath:      filepath.Join(dir, newName),
		}
	}

	// Two-pass conflict detection: mark ALL duplicates (not just second occurrence)
	nameCount := make(map[string]int)
	for _, p := range previews {
		nameCount[strings.ToLower(p.NewName)]++
	}
	for i := range previews {
		if nameCount[strings.ToLower(previews[i].NewName)] > 1 {
			previews[i].Conflict = true
		}
	}

	return previews, nil
}

// ExecuteRename performs the actual file renames. It skips conflicting entries.
func (s *RenamerService) ExecuteRename(previews []domain.RenamePreview) domain.RenameResult {
	var renamed int
	var errs []string

	for _, p := range previews {
		if p.Conflict {
			continue
		}
		if p.OriginalPath == p.NewPath {
			continue
		}

		if err := s.fs.Rename(p.OriginalPath, p.NewPath); err != nil {
			errs = append(errs, fmt.Sprintf("failed to rename %q: %v", p.OriginalName, err))
		} else {
			renamed++
		}
	}

	result := domain.RenameResult{
		RenamedCount: renamed,
		Errors:       errs,
	}

	if len(errs) == 0 {
		result.Success = true
		result.Message = fmt.Sprintf("Successfully renamed %d files", renamed)
	} else {
		result.Message = fmt.Sprintf("Renamed %d files with %d errors", renamed, len(errs))
	}

	return result
}
