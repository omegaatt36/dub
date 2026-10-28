package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	osfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

func TestUnicodeTargetCollisionPolicy(t *testing.T) {
	for _, pair := range [][2]string{{"x", "X"}, {"é", "e\u0301"}, {"Σ", "ς"}, {"ß", "SS"}} {
		t.Run(pair[0], func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, name := range []string{"1", "2"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
			}
			svc := NewRenamerService(&foldedFS{})
			files := []domain.FileItem{{Name: "1", Path: filepath.Join(dir, "1")}, {Name: "2", Path: filepath.Join(dir, "2")}}
			p, err := svc.PreviewRename(files, pair[:], port.UseProposedExtension)
			require.NoError(t, err)
			require.True(t, p[0].Conflict)
			require.True(t, p[1].Conflict)
			p[0].Conflict, p[1].Conflict = false, false
			result := svc.ExecuteRename(p)
			require.False(t, result.Success)
			require.False(t, result.RolledBack)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 2, "reject before creating a journal or staging")
		})
	}
}

func TestNativeUnicodeSourcesCannotInferSensitivity(t *testing.T) {
	for _, source := range []string{"ı", "İ"} {
		t.Run(source, func(t *testing.T) {
			dir := canonicalTempDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, source), nil, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "2"), nil, 0o600))
			d, err := (&osfs.OSFileSystem{}).OpenRenameDirectory(dir)
			require.NoError(t, err)
			sensitive, known := d.CaseSensitive()
			require.NoError(t, d.Close())
			if sensitive && known {
				t.Skip("requires insensitive or unknown filesystem")
			}
			p := safetyPreviews(t, dir, []string{source, "2"}, []string{"x", "X"})
			require.True(t, p[0].Conflict)
			require.True(t, p[1].Conflict)
		})
	}
}

func TestNativeNormalizationCollisionInDigitDirectory(t *testing.T) {
	dir := canonicalTempDir(t)
	for _, name := range []string{"1", "2"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	p := safetyPreviews(t, dir, []string{"1", "2"}, []string{"é", "e\u0301"})
	require.True(t, p[0].Conflict)
	require.True(t, p[1].Conflict)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

type lookupDirectory struct {
	port.RenameDirectory
	moves int
	path  string
}

func (d *lookupDirectory) EntryIdentity(name string) (string, error) {
	// Emulate a real normalization/case-insensitive lookup over a native dir.
	entries, err := os.ReadDir(d.path)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if nameKey(e.Name(), false) == nameKey(name, false) {
			return d.RenameDirectory.EntryIdentity(e.Name())
		}
	}
	return "", os.ErrNotExist
}

func (d *lookupDirectory) Rename(a, b string) error {
	d.moves++
	return d.RenameDirectory.Rename(a, b)
}

func TestRecoveryAliasOutsiderPreflightHasZeroMoves(t *testing.T) {
	for _, names := range [][2]string{{"a", "A"}, {"é", "e\u0301"}, {".dub-rename-temp", ".DUB-RENAME-TEMP"}} {
		t.Run(names[0], func(t *testing.T) {
			dir := canonicalTempDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), nil, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, names[1]), []byte("outside"), 0o600))
			session, err := (&osfs.OSFileSystem{}).OpenRenameDirectory(dir)
			require.NoError(t, err)
			defer session.Close()
			id, err := session.EntryIdentity("target")
			require.NoError(t, err)
			d := &lookupDirectory{RenameDirectory: session, path: dir}
			e := journalEntry{Original: names[0], Target: "target", Temp: ".dub-rename-temp", SourceID: id}
			if names[0] == ".dub-rename-temp" {
				e.Original = "original"
			}
			err = restoreJournal(d, renameJournal{Entries: []journalEntry{e}})
			require.Error(t, err)
			require.Zero(t, d.moves)
		})
	}
}

func TestJournalRejectsEquivalentDuplicateNames(t *testing.T) {
	dir := canonicalTempDir(t)
	session, err := (&foldedFS{}).OpenRenameDirectory(dir)
	require.NoError(t, err)
	defer session.Close()
	id, err := session.Identity()
	require.NoError(t, err)
	for _, pair := range [][2]string{{"a", "A"}, {"é", "e\u0301"}} {
		j := renameJournal{Version: 1, DirectoryID: id, Entries: []journalEntry{
			{Original: pair[0], Target: "1", Temp: ".dub-rename-1", SourceID: "1"},
			{Original: pair[1], Target: "2", Temp: ".dub-rename-2", SourceID: "2"},
		}}
		data, err := json.Marshal(j)
		require.NoError(t, err)
		_, err = decodeJournal(session, data)
		require.Error(t, err)
	}
}
