package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

type RenamerService struct{ fs port.FileSystem }

func NewRenamerService(fs port.FileSystem) *RenamerService {
	return &RenamerService{fs: fs}
}

func validateFileName(name string) error {
	if name == "" || name == "." || name == ".." ||
		domain.IsRenameInternal(name) ||
		strings.ContainsAny(name, "/\\\x00") {
		return domain.ErrInvalidFileName
	}
	return nil
}

func nameKey(name string, sensitive bool) string {
	if sensitive {
		// Canonical equivalents are conservatively rejected even on systems
		// that permit both spellings, including case-sensitive Linux.
		return norm.NFC.String(name)
	}
	// Conservative alias screening, not an emulation of filesystem lookup.
	return norm.NFC.String(cases.Fold().String(norm.NFD.String(name)))
}

func (s *RenamerService) PreviewRename(files []domain.FileItem, newNames []string, policy port.ExtensionPolicy) ([]domain.RenamePreview, error) {
	if len(files) != len(newNames) {
		return nil, domain.ErrMismatchedNames
	}
	previews := make([]domain.RenamePreview, len(files))
	for i, f := range files {
		name := strings.TrimSpace(newNames[i])
		switch {
		case name == "":
			name = f.Name
		case policy == port.KeepExtension && f.Extension != "" && !strings.HasSuffix(strings.ToLower(name), strings.ToLower(f.Extension)):
			name += f.Extension
		}
		previews[i] = domain.RenamePreview{
			Extension: f.Extension, Size: f.Size, OriginalName: f.Name, NewName: name,
			OriginalPath: f.Path, NewPath: filepath.Join(filepath.Dir(f.Path), name),
		}
	}
	dir, err := validatePaths(previews)
	if err != nil {
		return nil, err
	}
	sensitive := false
	var directory port.RenameDirectory
	var entries []os.DirEntry
	if len(previews) > 0 && s.fs != nil {
		if opener, ok := s.fs.(port.RenameDirectoryOpener); ok {
			d, err := opener.OpenRenameDirectory(dir)
			if err != nil {
				return nil, err
			}
			defer d.Close()
			directory = d
			id, err := d.Identity()
			if err != nil || id == "" {
				return nil, fmt.Errorf("cannot identify directory: %v", err)
			}
			for i := range previews {
				previews[i].DirectoryID = id
				previews[i].SourceID, err = d.EntryIdentity(previews[i].OriginalName)
				if err != nil {
					return nil, err
				}
			}
			value, known := d.CaseSensitive()
			sensitive = value && known
			entries, err = d.ReadDir()
			if err != nil {
				return nil, err
			}
		} else {
			entries, err = s.fs.ReadDir(dir)
			if err != nil {
				return nil, err
			}
		}
	}
	counts, selected, occupied, ownedIDs := map[string]int{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, p := range previews {
		counts[nameKey(p.NewName, sensitive)]++
		selected[nameKey(p.OriginalName, sensitive)] = true
		if p.SourceID != "" {
			ownedIDs[p.SourceID] = true
		}
	}
	for _, e := range entries {
		occupied[nameKey(e.Name(), sensitive)] = true
	}
	for i, p := range previews {
		key := nameKey(p.NewName, sensitive)
		previews[i].Conflict = counts[key] > 1 || (occupied[key] && !selected[key])
		if directory != nil {
			id, lookupErr := directory.EntryIdentity(p.NewName)
			if lookupErr == nil {
				previews[i].Conflict = previews[i].Conflict || !ownedIDs[id]
			} else if !os.IsNotExist(lookupErr) {
				return nil, lookupErr
			}
		}
		if p.OriginalName != p.NewName {
			previews[i].OriginalDiff, previews[i].NewDiff = domain.ComputeDiff(p.OriginalName, p.NewName)
		}
	}
	return previews, nil
}

func validatePaths(previews []domain.RenamePreview) (string, error) {
	dir := ""
	for _, p := range previews {
		if validateFileName(p.OriginalName) != nil || validateFileName(p.NewName) != nil {
			return "", domain.ErrInvalidFileName
		}
		d := filepath.Dir(p.OriginalPath)
		if !filepath.IsAbs(p.OriginalPath) || filepath.Clean(p.OriginalPath) != p.OriginalPath ||
			p.OriginalPath != filepath.Join(d, p.OriginalName) || p.NewPath != filepath.Join(d, p.NewName) ||
			(dir != "" && dir != d) {
			return "", domain.ErrInvalidPath
		}
		dir = d
	}
	return dir, nil
}

func renameFailure(err error) domain.RenameResult {
	return domain.RenameResult{Message: err.Error(), Errors: []string{err.Error()}}
}

func (s *RenamerService) ExecuteRename(previews []domain.RenamePreview) domain.RenameResult {
	dir, err := validatePaths(previews)
	if err != nil {
		return renameFailure(err)
	}
	if len(previews) == 0 {
		return domain.RenameResult{Success: true, Message: "Successfully renamed 0 files"}
	}
	d, err := s.openLocked(dir)
	if err != nil {
		return renameFailure(err)
	}
	defer d.Close()
	if _, err := d.ReadJournal(); !os.IsNotExist(err) {
		return renameFailure(fmt.Errorf("pending or unreadable rename journal; run recovery before renaming"))
	}
	id, err := d.Identity()
	if err != nil {
		return renameFailure(err)
	}
	value, known := d.CaseSensitive()
	sensitive := value && known
	sources, targets, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, p := range previews {
		source, target := nameKey(p.OriginalName, sensitive), nameKey(p.NewName, sensitive)
		if p.Conflict || sources[source] || targets[target] || p.DirectoryID == "" || p.DirectoryID != id || p.SourceID == "" || ids[p.SourceID] {
			return renameFailure(fmt.Errorf("conflicting or stale preview; generate a new preview"))
		}
		sources[source], targets[target], ids[p.SourceID] = true, true, true
		if err := verifyEntry(d, p.OriginalName, p.SourceID); err != nil {
			return renameFailure(err)
		}
	}
	entries, err := d.ReadDir()
	if err != nil {
		return renameFailure(err)
	}
	for _, e := range entries {
		key := nameKey(e.Name(), sensitive)
		if targets[key] && !sources[key] {
			return renameFailure(fmt.Errorf("target already exists: %q", e.Name()))
		}
	}
	for _, p := range previews {
		targetID, err := d.EntryIdentity(p.NewName)
		if err == nil && !ids[targetID] {
			return renameFailure(fmt.Errorf("target already exists: %q", p.NewName))
		}
		if err != nil && !os.IsNotExist(err) {
			return renameFailure(err)
		}
	}
	return executeJournal(d, id, previews)
}

func (s *RenamerService) openLocked(dir string) (port.RenameDirectory, error) {
	opener, ok := s.fs.(port.RenameDirectoryOpener)
	if !ok {
		return nil, fmt.Errorf("filesystem does not support safe rename sessions")
	}
	d, err := opener.OpenRenameDirectory(dir)
	if err != nil {
		return nil, err
	}
	if err := d.Lock(); err != nil {
		d.Close()
		return nil, fmt.Errorf("another rename or recovery is active: %w", err)
	}
	return d, nil
}

func verifyEntry(d port.RenameDirectory, name, want string) error {
	id, err := d.EntryIdentity(name)
	if err != nil {
		return fmt.Errorf("identify %q: %w", name, err)
	}
	if id != want || want == "" {
		return fmt.Errorf("source entry %q changed; refusing rename", name)
	}
	return nil
}
