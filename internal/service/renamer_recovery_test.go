package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	osfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

func safetyPreviews(t *testing.T, dir string, names, targets []string) []domain.RenamePreview {
	t.Helper()
	files := make([]domain.FileItem, len(names))
	for i, name := range names {
		files[i] = domain.FileItem{Name: name, Path: filepath.Join(dir, name)}
	}
	p, err := NewRenamerService(&osfs.OSFileSystem{}).PreviewRename(files, targets, port.UseProposedExtension)
	require.NoError(t, err)
	return p
}

func TestSourceReplacementRejectedForExecuteAndUndo(t *testing.T) {
	for _, undo := range []bool{false, true} {
		t.Run(fmt.Sprint(undo), func(t *testing.T) {
			dir := canonicalTempDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("owned"), 0o600))
			p := safetyPreviews(t, dir, []string{"a"}, []string{"b"})
			svc := NewRenamerService(&osfs.OSFileSystem{})
			if undo {
				require.True(t, svc.ExecuteRename(p).Success)
				p[0].OriginalName, p[0].NewName = p[0].NewName, p[0].OriginalName
				p[0].OriginalPath, p[0].NewPath = p[0].NewPath, p[0].OriginalPath
			}
			require.NoError(t, os.Rename(p[0].OriginalPath, filepath.Join(dir, "saved")))
			require.NoError(t, os.WriteFile(p[0].OriginalPath, []byte("foreign"), 0o600))
			result := svc.ExecuteRename(p)
			require.False(t, result.Success)
			data, err := os.ReadFile(p[0].OriginalPath)
			require.NoError(t, err)
			require.Equal(t, "foreign", string(data))
		})
	}
}

func TestReplacementDuringStagingIsRestored(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("owned"), 0o600))
	p := safetyPreviews(t, dir, []string{"a"}, []string{"b"})
	fs := &injectedFS{before: func(call int, from, to string) error {
		if call == 1 {
			require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "saved")))
			return os.WriteFile(filepath.Join(dir, "a"), []byte("replacement"), 0o600)
		}
		return nil
	}}
	result := NewRenamerService(fs).ExecuteRename(p)
	require.False(t, result.Success)
	require.True(t, result.RolledBack)
	require.Empty(t, result.RollbackErrors)
	data, err := os.ReadFile(filepath.Join(dir, "a"))
	require.NoError(t, err)
	require.Equal(t, "replacement", string(data))
}

// Panic simulates process interruption: execution unwinds without rollback, then
// a new service/session recovers solely from the persisted intent and entries.
func TestRecoveryAfterEachRenameBoundary(t *testing.T) {
	for stop := 1; stop <= 5; stop++ {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, name := range []string{"a", "b"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
			}
			p := safetyPreviews(t, dir, []string{"a", "b"}, []string{"b", "a"})
			fs := &crashFS{stop: stop}
			require.Panics(t, func() { NewRenamerService(fs).ExecuteRename(p) })
			svc := NewRenamerService(&osfs.OSFileSystem{})
			pending, err := svc.RecoveryPending(dir)
			require.NoError(t, err)
			require.True(t, pending)
			require.False(t, svc.ExecuteRename(p).Success)
			result := svc.RecoverRename(dir)
			require.True(t, result.Success, result)
			require.True(t, result.RolledBack)
			for _, name := range []string{"a", "b"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				require.NoError(t, err)
				require.Equal(t, name, string(data))
			}
			pending, err = svc.RecoveryPending(dir)
			require.NoError(t, err)
			require.False(t, pending)
		})
	}
}

type crashFS struct {
	osfs.OSFileSystem
	stop int
}

func (f *crashFS) OpenRenameDirectory(path string) (port.RenameDirectory, error) {
	d, err := f.OSFileSystem.OpenRenameDirectory(path)
	if err != nil {
		return nil, err
	}
	return &crashDirectory{RenameDirectory: d, stop: f.stop}, nil
}

type crashDirectory struct {
	port.RenameDirectory
	calls int
	stop  int
}

func (d *crashDirectory) Rename(a, b string) error {
	d.calls++
	err := d.RenameDirectory.Rename(a, b)
	if err == nil && d.calls == d.stop {
		panic("process interrupted")
	}
	return err
}

func (d *crashDirectory) RemoveJournal() error {
	if d.stop == 5 {
		panic("process interrupted before cleanup")
	}
	return d.RenameDirectory.RemoveJournal()
}

func TestRecoveryRefusesAmbiguousState(t *testing.T) {
	for _, scenario := range []string{"malformed", "wrong directory", "outside original", "replaced temp", "hardlink alias", "duplicate entries"} {
		t.Run(scenario, func(t *testing.T) {
			dir := canonicalTempDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("owned"), 0o600))
			p := safetyPreviews(t, dir, []string{"a"}, []string{"b"})
			d, err := (&osfs.OSFileSystem{}).OpenRenameDirectory(dir)
			require.NoError(t, err)
			defer d.Close()
			j := renameJournal{Version: 1, DirectoryID: p[0].DirectoryID, Entries: []journalEntry{{Original: "a", Target: "b", Temp: ".dub-rename-test", SourceID: p[0].SourceID}}}
			require.NoError(t, d.Rename("a", ".dub-rename-test"))
			switch scenario {
			case "wrong directory":
				j.DirectoryID = "foreign"
			case "outside original":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("outside"), 0o600))
			case "replaced temp":
				require.NoError(t, os.Rename(filepath.Join(dir, ".dub-rename-test"), filepath.Join(dir, "saved")))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".dub-rename-test"), []byte("outside"), 0o600))
			case "hardlink alias":
				require.NoError(t, os.Link(filepath.Join(dir, ".dub-rename-test"), filepath.Join(dir, "b")))
			case "duplicate entries":
				j.Entries = append(j.Entries, j.Entries[0])
			}
			data, err := json.Marshal(j)
			require.NoError(t, err)
			if scenario == "malformed" {
				data = []byte("{")
			}
			require.NoError(t, d.CreateJournal(data))
			svc := NewRenamerService(&osfs.OSFileSystem{})
			require.False(t, svc.RecoverRename(dir).Success)
			require.False(t, svc.ExecuteRename(p).Success)
			require.FileExists(t, filepath.Join(dir, ".dub-rename-journal"))
		})
	}
}

func TestCaseSensitivityObservedWithoutPreviewWrites(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o600))
	d, err := (&osfs.OSFileSystem{}).OpenRenameDirectory(dir)
	require.NoError(t, err)
	sensitive, known := d.CaseSensitive()
	require.NoError(t, d.Close())
	if !known {
		require.False(t, sensitive, "unknown capability must be conservative")
	}
	p := safetyPreviews(t, dir, []string{"a"}, []string{"A"})
	require.False(t, p[0].Conflict, "case-only rename is valid")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "preview must not create probes")
	if sensitive {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "A"), []byte("A"), 0o600))
		p = safetyPreviews(t, dir, []string{"a", "A"}, []string{"x", "X"})
		require.False(t, p[0].Conflict)
		require.False(t, p[1].Conflict)
		result := NewRenamerService(&osfs.OSFileSystem{}).ExecuteRename(p)
		require.True(t, result.Success, result)
	}
}

func TestDirectoryLockAndReservedNames(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), nil, 0o600))
	p := safetyPreviews(t, dir, []string{"a"}, []string{"b"})
	d, err := (&osfs.OSFileSystem{}).OpenRenameDirectory(dir)
	require.NoError(t, err)
	defer d.Close()
	require.NoError(t, d.Lock())
	svc := NewRenamerService(&osfs.OSFileSystem{})
	require.False(t, svc.ExecuteRename(p).Success)
	require.False(t, svc.RecoverRename(dir).Success)
	for _, name := range []string{".dub-rename-journal", ".dub-rename-test", ".DUB-RENAME-test"} {
		require.Error(t, validateFileName(name))
	}
}

func TestRecoveryCanRestartDuringRecovery(t *testing.T) {
	for stop := 1; stop <= 4; stop++ {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, name := range []string{"a", "b"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
			}
			p := safetyPreviews(t, dir, []string{"a", "b"}, []string{"b", "a"})
			require.Panics(t, func() { NewRenamerService(&crashFS{stop: 5}).ExecuteRename(p) })
			require.Panics(t, func() { NewRenamerService(&crashFS{stop: stop}).RecoverRename(dir) })
			result := NewRenamerService(&osfs.OSFileSystem{}).RecoverRename(dir)
			require.True(t, result.Success, result)
			for _, name := range []string{"a", "b"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				require.NoError(t, err)
				require.Equal(t, name, string(data))
			}
		})
	}
}

type foldedFS struct {
	osfs.OSFileSystem
	known bool
}

func (f *foldedFS) OpenRenameDirectory(path string) (port.RenameDirectory, error) {
	d, err := f.OSFileSystem.OpenRenameDirectory(path)
	if err != nil {
		return nil, err
	}
	return &foldedDirectory{RenameDirectory: d, known: f.known}, nil
}

type foldedDirectory struct {
	port.RenameDirectory
	known bool
}

func (d *foldedDirectory) CaseSensitive() (bool, bool) { return false, d.known }

func TestInsensitiveAndUnknownCollisionPolicy(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(fmt.Sprint(known), func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, name := range []string{"a", "b"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
			}
			svc := NewRenamerService(&foldedFS{known: known})
			files := []domain.FileItem{{Name: "a", Path: filepath.Join(dir, "a")}, {Name: "b", Path: filepath.Join(dir, "b")}}
			p, err := svc.PreviewRename(files, []string{"x", "X"}, port.UseProposedExtension)
			require.NoError(t, err)
			require.True(t, p[0].Conflict)
			require.True(t, p[1].Conflict)
			// Execution rechecks policy rather than trusting Conflict flags.
			p[0].Conflict, p[1].Conflict = false, false
			require.False(t, svc.ExecuteRename(p).Success)
			p, err = svc.PreviewRename(files[:1], []string{"A"}, port.UseProposedExtension)
			require.NoError(t, err)
			require.False(t, p[0].Conflict)
			require.True(t, svc.ExecuteRename(p).Success)
		})
	}
}

func TestSymlinkJournalNeverFollowed(t *testing.T) {
	dir := canonicalTempDir(t)
	outside := filepath.Join(canonicalTempDir(t), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".dub-rename-journal")))
	svc := NewRenamerService(&osfs.OSFileSystem{})
	pending, err := svc.RecoveryPending(dir)
	require.Error(t, err)
	require.True(t, pending)
	require.False(t, svc.RecoverRename(dir).Success)
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
}
