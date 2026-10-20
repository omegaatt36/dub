package service

import (
	"crypto/rand"
	"fmt"
	"os"
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

// validateFileName rejects names that cannot exist on disk or would escape the
// target directory. Extension characters are included so a proposed name cannot
// smuggle a path separator past the join.
func validateFileName(name string) error {
	if name == "" {
		return domain.ErrInvalidFileName
	}
	if (name == "." || name == "..") ||
		strings.ContainsAny(name, `/\`) ||
		strings.ContainsRune(name, 0) {
		return domain.ErrInvalidFileName
	}
	return nil
}

// PreviewRename generates rename previews for the given files and new names,
// applying policy to decide each resulting extension, and flags conflicts.
func (s *RenamerService) PreviewRename(files []domain.FileItem, newNames []string, policy port.ExtensionPolicy) ([]domain.RenamePreview, error) {
	if len(files) != len(newNames) {
		return nil, domain.ErrMismatchedNames
	}

	previews := make([]domain.RenamePreview, len(files))
	for i, f := range files {
		newName := strings.TrimSpace(newNames[i])
		switch {
		case newName == "":
			newName = f.Name
		case policy == port.KeepExtension && f.Extension != "" && !strings.HasSuffix(strings.ToLower(newName), strings.ToLower(f.Extension)):
			newName = newName + f.Extension
		}

		if err := validateFileName(f.Name); err != nil {
			return nil, err
		}
		{
			if err := validateFileName(newName); err != nil {
				return nil, fmt.Errorf("invalid name %q: %w", newName, domain.ErrInvalidFileName)
			}
		}

		dir := filepath.Dir(f.Path)
		previews[i] = domain.RenamePreview{
			Extension:    f.Extension,
			Size:         f.Size,
			OriginalName: f.Name,
			NewName:      newName,
			OriginalPath: f.Path,
			NewPath:      filepath.Join(dir, newName),
		}
	}

	previewDir := ""
	for _, p := range previews {
		dir, err := validatePreviews([]domain.RenamePreview{p})
		if err != nil {
			return nil, err
		}
		if previewDir != "" && dir != previewDir {
			return nil, domain.ErrInvalidPath
		}
		previewDir = dir
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

	if s.fs != nil && len(previews) > 0 {
		var entries []os.DirEntry
		if opener, ok := s.fs.(port.RenameDirectoryOpener); ok {
			session, err := opener.OpenRenameDirectory(previewDir)
			if err != nil {
				return nil, err
			}
			defer session.Close()
			identity, err := session.Identity()
			if err != nil {
				return nil, err
			}
			if identity == "" {
				return nil, fmt.Errorf("empty directory identity")
			}
			entries, err = session.ReadDir()
			if err != nil {
				return nil, err
			}
			for i := range previews {
				previews[i].DirectoryID = identity
			}
		} else {
			var err error
			entries, err = s.fs.ReadDir(previewDir)
			if err != nil {
				return nil, err
			}
		}
		selected := map[string]bool{}
		for _, p := range previews {
			selected[strings.ToLower(p.OriginalName)] = true
		}
		occupied := map[string]bool{}
		for _, e := range entries {
			occupied[strings.ToLower(e.Name())] = true
		}
		for i, p := range previews {
			if occupied[strings.ToLower(p.NewName)] && !selected[strings.ToLower(p.NewName)] {
				previews[i].Conflict = true
			}
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

func validatePreviews(previews []domain.RenamePreview) (string, error) {
	dir := ""
	sources, targets := map[string]bool{}, map[string]bool{}
	for _, p := range previews {
		if validateFileName(p.OriginalName) != nil || validateFileName(p.NewName) != nil {
			return "", domain.ErrInvalidFileName
		}
		d := filepath.Dir(p.OriginalPath)
		if !filepath.IsAbs(p.OriginalPath) || filepath.Clean(p.OriginalPath) != p.OriginalPath || p.OriginalPath != filepath.Join(d, p.OriginalName) || p.NewPath != filepath.Join(d, p.NewName) {
			return "", domain.ErrInvalidPath
		}
		if dir != "" && dir != d {
			return "", domain.ErrInvalidPath
		}
		dir = d
		source, target := strings.ToLower(p.OriginalName), strings.ToLower(p.NewName)
		if sources[source] || targets[target] || p.Conflict {
			return "", fmt.Errorf("conflicting rename: %q", p.NewName)
		}
		sources[source] = true
		targets[target] = true
	}
	return dir, nil
}

func (s *RenamerService) ExecuteRename(previews []domain.RenamePreview) domain.RenameResult {
	fail := func(err error) domain.RenameResult {
		return domain.RenameResult{Message: err.Error(), Errors: []string{err.Error()}}
	}
	dir, err := validatePreviews(previews)
	if err != nil {
		return fail(err)
	}
	if len(previews) == 0 {
		return domain.RenameResult{Success: true, Message: "Successfully renamed 0 files"}
	}
	opener, ok := s.fs.(port.RenameDirectoryOpener)
	if !ok {
		return fail(fmt.Errorf("filesystem does not support safe rename sessions"))
	}
	session, err := opener.OpenRenameDirectory(dir)
	if err != nil {
		return fail(err)
	}
	defer session.Close()
	identity, err := session.Identity()
	if err != nil {
		return fail(err)
	}
	for _, p := range previews {
		if p.DirectoryID == "" || p.DirectoryID != identity {
			return fail(fmt.Errorf("directory changed since preview; generate a new preview"))
		}
	}
	entries, err := session.ReadDir()
	if err != nil {
		return fail(err)
	}
	occupied := map[string]bool{}
	for _, e := range entries {
		occupied[strings.ToLower(e.Name())] = true
	}
	selected := map[string]bool{}
	for _, p := range previews {
		selected[strings.ToLower(p.OriginalName)] = true
	}
	var changes []domain.RenamePreview
	for _, p := range previews {
		if !occupied[strings.ToLower(p.OriginalName)] {
			return fail(fmt.Errorf("source missing: %q", p.OriginalName))
		}
		if occupied[strings.ToLower(p.NewName)] && !selected[strings.ToLower(p.NewName)] {
			return fail(fmt.Errorf("target already exists: %q", p.NewName))
		}
		if p.OriginalName != p.NewName {
			changes = append(changes, p)
		}
	}
	type move struct{ from, to string }
	var moves []move
	rollback := func(cause error) domain.RenameResult {
		result := fail(cause)
		result.RolledBack = true
		for i := len(moves) - 1; i >= 0; i-- {
			m := moves[i]
			if err := session.Rename(m.to, m.from); err != nil {
				result.RollbackErrors = append(result.RollbackErrors, fmt.Sprintf("restore %q from %q: %v", m.from, m.to, err))
			}
		}
		return result
	}
	temps := make([]string, len(changes))
	for i, p := range changes {
		temporary := ".dub-rename-" + rand.Text()
		temps[i] = temporary
		if err := session.Rename(p.OriginalName, temporary); err != nil {
			return rollback(fmt.Errorf("stage %q: %w", p.OriginalName, err))
		}
		moves = append(moves, move{p.OriginalName, temporary})
	}
	for i, p := range changes {
		if err := session.Rename(temps[i], p.NewName); err != nil {
			return rollback(fmt.Errorf("rename %q: %w", p.OriginalName, err))
		}
		moves = append(moves, move{temps[i], p.NewName})
	}
	return domain.RenameResult{Success: true, RenamedCount: len(changes), Message: fmt.Sprintf("Successfully renamed %d files", len(changes))}
}
