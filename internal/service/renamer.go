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

// validateFileName checks for invalid characters in filenames.
func validateFileName(name string) error {
	if strings.Contains(name, "..") ||
		strings.Contains(name, "/") ||
		strings.Contains(name, "\\") {
		return domain.ErrInvalidFileName
	}
	return nil
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

		if newName != f.Name {
			if err := validateFileName(newName); err != nil {
				return nil, fmt.Errorf("invalid name %q: %w", newName, err)
			}
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

	// Compute diffs for changed files
	for i := range previews {
		if previews[i].OriginalName != previews[i].NewName {
			previews[i].OriginalDiff, previews[i].NewDiff = domain.ComputeDiff(
				previews[i].OriginalName, previews[i].NewName,
			)
		}
	}

	return previews, nil
}

// ExecuteRename performs the actual file renames with rollback on failure.
// If any rename fails, all previously completed renames are reversed.
func (s *RenamerService) ExecuteRename(previews []domain.RenamePreview) domain.RenameResult {
	var completed []domain.RenamePreview

	for _, p := range previews {
		if p.Conflict {
			continue
		}
		if p.OriginalPath == p.NewPath {
			continue
		}

		if err := s.fs.Rename(p.OriginalPath, p.NewPath); err != nil {
			// Rollback all completed renames in reverse order
			var rollbackErrors []string
			for i := len(completed) - 1; i >= 0; i-- {
				c := completed[i]
				if rbErr := s.fs.Rename(c.NewPath, c.OriginalPath); rbErr != nil {
					rollbackErrors = append(rollbackErrors, fmt.Sprintf("failed to rollback %q: %v", c.NewName, rbErr))
				}
			}

			return domain.RenameResult{
				Success:        false,
				RenamedCount:   0,
				Message:        fmt.Sprintf("Rename failed at %q: %v. Rolled back %d files.", p.OriginalName, err, len(completed)),
				Errors:         []string{fmt.Sprintf("failed to rename %q: %v", p.OriginalName, err)},
				RolledBack:     true,
				RollbackErrors: rollbackErrors,
			}
		}

		completed = append(completed, p)
	}

	return domain.RenameResult{
		Success:      true,
		RenamedCount: len(completed),
		Message:      fmt.Sprintf("Successfully renamed %d files", len(completed)),
	}
}
