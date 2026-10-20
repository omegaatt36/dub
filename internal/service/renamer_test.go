package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	osfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/mock"
	"github.com/omegaatt36/dub/internal/port"
)

func TestRenamerService_PreviewRename(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mock.NewMockFileSystem(ctrl)
	mockFS.EXPECT().ReadDir(gomock.Any()).Return(nil, nil).AnyTimes()
	svc := NewRenamerService(mockFS)

	t.Run("generates previews with extensions", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "old1.txt", Path: "/dir/old1.txt", Extension: ".txt"},
			{Name: "old2.txt", Path: "/dir/old2.txt", Extension: ".txt"},
		}
		names := []string{"new1", "new2"}

		previews, err := svc.PreviewRename(files, names, port.KeepExtension)
		require.NoError(t, err)
		require.Len(t, previews, 2)
		assert.Equal(t, "new1.txt", previews[0].NewName)
		assert.Equal(t, "new2.txt", previews[1].NewName)
	})

	t.Run("previews carry the source file's size and extension", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.jpg", Path: "/dir/a.jpg", Extension: ".jpg", Size: 42},
		}

		previews, err := svc.PreviewRename(files, []string{"b"}, port.KeepExtension)
		require.NoError(t, err)
		assert.Equal(t, ".jpg", previews[0].Extension)
		assert.Equal(t, uint64(42), previews[0].Size)
	})

	t.Run("preserves extension if already present", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "old.txt", Path: "/dir/old.txt", Extension: ".txt"},
		}
		names := []string{"new.txt"}

		previews, err := svc.PreviewRename(files, names, port.KeepExtension)
		require.NoError(t, err)
		assert.Equal(t, "new.txt", previews[0].NewName, "should not double extension")
	})

	t.Run("detects conflicts (all duplicates marked)", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
			{Name: "b.txt", Path: "/dir/b.txt", Extension: ".txt"},
			{Name: "c.txt", Path: "/dir/c.txt", Extension: ".txt"},
		}
		names := []string{"same", "same", "unique"}

		previews, err := svc.PreviewRename(files, names, port.KeepExtension)
		require.NoError(t, err)
		assert.True(t, previews[0].Conflict, "first duplicate should be conflict")
		assert.True(t, previews[1].Conflict, "second duplicate should be conflict")
		assert.False(t, previews[2].Conflict, "unique name should not be conflict")
	})

	t.Run("mismatched count returns error", func(t *testing.T) {
		files := []domain.FileItem{{Name: "a.txt"}}
		names := []string{"new1", "new2"}

		_, err := svc.PreviewRename(files, names, port.KeepExtension)
		assert.ErrorIs(t, err, domain.ErrMismatchedNames)
	})

	t.Run("empty new name uses original", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "keep.txt", Path: "/dir/keep.txt", Extension: ".txt"},
		}
		names := []string{""}

		previews, err := svc.PreviewRename(files, names, port.KeepExtension)
		require.NoError(t, err)
		assert.Equal(t, "keep.txt", previews[0].NewName, "empty name should keep original")
	})

	t.Run("rejects path traversal in new names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
		}
		names := []string{"../evil"}

		_, err := svc.PreviewRename(files, names, port.KeepExtension)
		assert.ErrorIs(t, err, domain.ErrInvalidFileName)
	})

	t.Run("rejects slash in new names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
		}
		names := []string{"sub/file"}

		_, err := svc.PreviewRename(files, names, port.KeepExtension)
		assert.ErrorIs(t, err, domain.ErrInvalidFileName)
	})

	t.Run("rejects backslash in new names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
		}
		names := []string{"sub\\file"}

		_, err := svc.PreviewRename(files, names, port.KeepExtension)
		assert.ErrorIs(t, err, domain.ErrInvalidFileName)
	})

	t.Run("computes diff segments for changed names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "photo_001.txt", Path: "/dir/photo_001.txt", Extension: ".txt"},
		}
		names := []string{"vacation_001"}

		previews, err := svc.PreviewRename(files, names, port.KeepExtension)
		require.NoError(t, err)
		require.NotNil(t, previews[0].OriginalDiff)
		require.NotNil(t, previews[0].NewDiff)

		// Verify reconstructed text is correct
		var origText, newText string
		for _, seg := range previews[0].OriginalDiff {
			origText += seg.Text
		}
		for _, seg := range previews[0].NewDiff {
			newText += seg.Text
		}
		assert.Equal(t, "photo_001.txt", origText)
		assert.Equal(t, "vacation_001.txt", newText)

		// Should have delete segments in original
		hasDelete := false
		for _, seg := range previews[0].OriginalDiff {
			if seg.Type == domain.DiffDelete {
				hasDelete = true
			}
		}
		assert.True(t, hasDelete, "should have delete segments")

		// Should have insert segments in new
		hasInsert := false
		for _, seg := range previews[0].NewDiff {
			if seg.Type == domain.DiffInsert {
				hasInsert = true
			}
		}
		assert.True(t, hasInsert, "should have insert segments")
	})

	t.Run("no diff for unchanged names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "keep.txt", Path: "/dir/keep.txt", Extension: ".txt"},
		}
		names := []string{""}

		previews, err := svc.PreviewRename(files, names, port.KeepExtension)
		require.NoError(t, err)
		assert.Nil(t, previews[0].OriginalDiff, "unchanged name should have no diff")
		assert.Nil(t, previews[0].NewDiff, "unchanged name should have no diff")
	})
}

func TestExecuteRenameSafety(t *testing.T) {
	for _, tc := range []struct {
		name             string
		sources, targets []string
		occupied         bool
		want             bool
	}{
		{"unselected target", []string{"a"}, []string{"b"}, true, false},
		{"swap", []string{"a", "b"}, []string{"b", "a"}, false, true},
		{"chain", []string{"a", "b"}, []string{"b", "c"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, n := range tc.sources {
				require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte(n), 0o600))
			}
			if tc.occupied {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("untouched"), 0o600))
			}
			previews := make([]domain.RenamePreview, len(tc.sources))
			for i, n := range tc.sources {
				previews[i] = domain.RenamePreview{OriginalName: n, NewName: tc.targets[i], OriginalPath: filepath.Join(dir, n), NewPath: filepath.Join(dir, tc.targets[i])}
			}
			bindDirectoryIdentity(t, dir, previews)
			result := NewRenamerService(&osfs.OSFileSystem{}).ExecuteRename(previews)
			require.Equal(t, tc.want, result.Success, result)
			if tc.want {
				for i, n := range tc.targets {
					data, err := os.ReadFile(filepath.Join(dir, n))
					require.NoError(t, err)
					require.Equal(t, tc.sources[i], string(data))
				}
			} else {
				data, err := os.ReadFile(filepath.Join(dir, "b"))
				require.NoError(t, err)
				require.Equal(t, "untouched", string(data))
				require.FileExists(t, filepath.Join(dir, "a"))
			}
		})
	}
}

func TestExecuteRejectsTamperedPreview(t *testing.T) {
	for _, name := range []string{"../outside", "~/outside", "/outside", `bad\name`, "bad\x00name", ".", "..", ""} {
		t.Run(name, func(t *testing.T) {
			dir := canonicalTempDir(t)
			original := filepath.Join(dir, "a")
			require.NoError(t, os.WriteFile(original, []byte("a"), 0o600))
			result := NewRenamerService(&osfs.OSFileSystem{}).ExecuteRename([]domain.RenamePreview{{OriginalName: "a", NewName: name, OriginalPath: original, NewPath: filepath.Join(dir, "b")}})
			require.False(t, result.Success)
			require.FileExists(t, original)
		})
	}
}

func TestPreviewExistingTargetAndDotNames(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), nil, 0o600))
	require.NoError(t, os.Symlink("missing", filepath.Join(dir, "b")))
	svc := NewRenamerService(&osfs.OSFileSystem{})
	files := []domain.FileItem{{Name: "a", Path: filepath.Join(dir, "a")}}
	previews, err := svc.PreviewRename(files, []string{"b"}, port.UseProposedExtension)
	require.NoError(t, err)
	require.True(t, previews[0].Conflict)
	_, err = svc.PreviewRename(files, []string{"photo..backup"}, port.UseProposedExtension)
	require.NoError(t, err)
}

// TestPreviewRenameExtensionPolicy locks in the two extension behaviours. The
// UseProposed case is the bug this policy exists for: "photo_1.webp" on a .jpg
// used to become "photo_1.webp.jpg".
func TestPreviewRenameExtensionPolicy(t *testing.T) {
	files := []domain.FileItem{
		{Name: "photo.jpg", Path: "/dir/photo.jpg", Extension: ".jpg"},
	}

	tests := []struct {
		name     string
		policy   port.ExtensionPolicy
		proposed string
		want     string
	}{
		{
			name:     "keep appends the original extension",
			policy:   port.KeepExtension,
			proposed: "trip_1.webp",
			want:     "trip_1.webp.jpg",
		},
		{
			name:     "keep does not double an identical extension",
			policy:   port.KeepExtension,
			proposed: "trip_1.jpg",
			want:     "trip_1.jpg",
		},
		{
			name:     "keep is case insensitive about the suffix",
			policy:   port.KeepExtension,
			proposed: "trip_1.JPG",
			want:     "trip_1.JPG",
		},
		{
			name:     "proposed honours a new extension",
			policy:   port.UseProposedExtension,
			proposed: "trip_1.webp",
			want:     "trip_1.webp",
		},
		{
			name:     "proposed leaves an extensionless name alone",
			policy:   port.UseProposedExtension,
			proposed: "trip_1",
			want:     "trip_1",
		},
		{
			name:     "empty proposal always keeps the original name",
			policy:   port.UseProposedExtension,
			proposed: "   ",
			want:     "photo.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewRenamerService(nil)
			previews, err := svc.PreviewRename(files, []string{tt.proposed}, tt.policy)
			require.NoError(t, err)
			require.Len(t, previews, 1)
			assert.Equal(t, tt.want, previews[0].NewName)
		})
	}
}

// TestPreviewRenameRejectsUnsafeNames guards path traversal and empty names,
// which matter more once a user can type an arbitrary extension.
func TestPreviewRenameRejectsUnsafeNames(t *testing.T) {
	tests := []struct {
		name     string
		proposed string
	}{
		{name: "parent traversal", proposed: "../escape"},
		{name: "forward slash", proposed: "sub/dir"},
		{name: "backslash", proposed: `sub\dir`},
		{name: "nul byte", proposed: "bad\x00name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := []domain.FileItem{{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"}}
			svc := NewRenamerService(nil)
			_, err := svc.PreviewRename(files, []string{tt.proposed}, port.UseProposedExtension)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidFileName)
		})
	}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

type injectedDirectory struct {
	port.RenameDirectory
	calls  int
	before func(int, string, string) error
}

func (d *injectedDirectory) Rename(a, b string) error {
	d.calls++
	if d.before != nil {
		if err := d.before(d.calls, a, b); err != nil {
			return err
		}
	}
	return d.RenameDirectory.Rename(a, b)
}

type injectedFS struct {
	osfs.OSFileSystem
	before func(int, string, string) error
}

func (f *injectedFS) OpenRenameDirectory(path string) (port.RenameDirectory, error) {
	d, err := f.OSFileSystem.OpenRenameDirectory(path)
	if err != nil {
		return nil, err
	}
	return &injectedDirectory{RenameDirectory: d, before: f.before}, nil
}

func TestExecuteRollbackAndConcurrentTarget(t *testing.T) {
	for _, scenario := range []string{"stage failure", "commit failure", "concurrent target", "rollback failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, n := range []string{"a", "b"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte(n), 0o600))
			}
			previews := []domain.RenamePreview{{OriginalName: "a", NewName: "b", OriginalPath: filepath.Join(dir, "a"), NewPath: filepath.Join(dir, "b")}, {OriginalName: "b", NewName: "c", OriginalPath: filepath.Join(dir, "b"), NewPath: filepath.Join(dir, "c")}}
			fs := &injectedFS{before: func(call int, a, b string) error {
				if scenario == "stage failure" && call == 2 {
					return fmt.Errorf("injected stage failure")
				}
				if (scenario == "commit failure" || scenario == "rollback failure") && call == 4 {
					return fmt.Errorf("injected commit failure")
				}
				if scenario == "rollback failure" && call == 5 {
					return fmt.Errorf("injected rollback failure")
				}
				if scenario == "concurrent target" && call == 4 {
					return os.WriteFile(filepath.Join(dir, "c"), []byte("competitor"), 0o600)
				}
				return nil
			}}
			bindDirectoryIdentity(t, dir, previews)
			result := NewRenamerService(fs).ExecuteRename(previews)
			require.False(t, result.Success)
			require.True(t, result.RolledBack)
			if scenario == "rollback failure" {
				require.NotEmpty(t, result.RollbackErrors)
				return
			}
			require.Empty(t, result.RollbackErrors)
			for _, n := range []string{"a", "b"} {
				data, err := os.ReadFile(filepath.Join(dir, n))
				require.NoError(t, err)
				require.Equal(t, n, string(data))
			}
			if scenario == "concurrent target" {
				data, err := os.ReadFile(filepath.Join(dir, "c"))
				require.NoError(t, err)
				require.Equal(t, "competitor", string(data))
			}
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			for _, e := range entries {
				require.NotContains(t, e.Name(), ".dub-rename-")
			}
		})
	}
}

func TestExecuteSymlinkAndPathValidation(t *testing.T) {
	t.Run("renames link itself", func(t *testing.T) {
		dir := canonicalTempDir(t)
		outside := filepath.Join(canonicalTempDir(t), "outside")
		require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "a")))
		p := domain.RenamePreview{OriginalName: "a", NewName: "b", OriginalPath: filepath.Join(dir, "a"), NewPath: filepath.Join(dir, "b")}
		rows := []domain.RenamePreview{p}
		bindDirectoryIdentity(t, dir, rows)
		require.True(t, NewRenamerService(&osfs.OSFileSystem{}).ExecuteRename(rows).Success)
		link, err := os.Readlink(p.NewPath)
		require.NoError(t, err)
		require.Equal(t, outside, link)
		data, err := os.ReadFile(outside)
		require.NoError(t, err)
		require.Equal(t, "outside", string(data))
	})
	t.Run("rejects any invalid row before mutation", func(t *testing.T) {
		for _, kind := range []string{"different directory", "path mismatch", "noncanonical", "relative", "invalid original", "conflict"} {
			t.Run(kind, func(t *testing.T) {
				dir := canonicalTempDir(t)
				outside := canonicalTempDir(t)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(outside, "x"), []byte("x"), 0o600))
				first := domain.RenamePreview{OriginalName: "a", NewName: "b", OriginalPath: filepath.Join(dir, "a"), NewPath: filepath.Join(dir, "b")}
				second := domain.RenamePreview{OriginalName: "x", NewName: "y", OriginalPath: filepath.Join(outside, "x"), NewPath: filepath.Join(outside, "y")}
				switch kind {
				case "path mismatch":
					second = first
					second.NewPath = filepath.Join(outside, "b")
				case "noncanonical":
					second = first
					second.OriginalPath = dir + "/./a"
				case "relative":
					second = first
					second.OriginalPath = "a"
				case "invalid original":
					second = first
					second.OriginalName = "../a"
				case "conflict":
					second = first
					second.Conflict = true
				}
				result := NewRenamerService(&osfs.OSFileSystem{}).ExecuteRename([]domain.RenamePreview{first, second})
				require.False(t, result.Success)
				require.FileExists(t, first.OriginalPath)
				require.FileExists(t, filepath.Join(outside, "x"))
			})
		}
	})
}

func TestDirectoryAliasCannotRetargetPreviewOrUndo(t *testing.T) {
	for _, undo := range []bool{false, true} {
		t.Run(fmt.Sprintf("undo=%t", undo), func(t *testing.T) {
			root := canonicalTempDir(t)
			a, b := filepath.Join(root, "A"), filepath.Join(root, "B")
			require.NoError(t, os.Mkdir(a, 0o700))
			require.NoError(t, os.Mkdir(b, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(a, "a"), []byte("owned"), 0o600))
			alias := filepath.Join(root, "alias")
			require.NoError(t, os.Symlink(a, alias))
			svc := NewRenamerService(&osfs.OSFileSystem{})
			previews, err := svc.PreviewRename([]domain.FileItem{{Name: "a", Path: filepath.Join(alias, "a")}}, []string{"b"}, port.UseProposedExtension)
			require.NoError(t, err)
			foreignName := "a"
			if undo {
				require.True(t, svc.ExecuteRename(previews).Success)
				foreignName = "b"
				for i := range previews {
					p := &previews[i]
					p.OriginalName, p.NewName = p.NewName, p.OriginalName
					p.OriginalPath, p.NewPath = p.NewPath, p.OriginalPath
				}
			}
			require.NoError(t, os.WriteFile(filepath.Join(b, foreignName), []byte("foreign"), 0o600))
			require.NoError(t, os.Remove(alias))
			require.NoError(t, os.Symlink(b, alias))
			result := svc.ExecuteRename(previews)
			require.False(t, result.Success, "retargeted alias must fail before any mutation")
			data, err := os.ReadFile(filepath.Join(b, foreignName))
			require.NoError(t, err)
			require.Equal(t, "foreign", string(data))
		})
	}
}

func bindDirectoryIdentity(t *testing.T, dir string, rows []domain.RenamePreview) {
	t.Helper()
	session, err := (&osfs.OSFileSystem{}).OpenRenameDirectory(dir)
	require.NoError(t, err)
	defer session.Close()
	id, err := session.Identity()
	require.NoError(t, err)
	for i := range rows {
		rows[i].DirectoryID = id
	}
}

func TestExecuteRequiresEveryDirectoryIdentity(t *testing.T) {
	for _, identity := range []string{"", "forged"} {
		t.Run(identity, func(t *testing.T) {
			dir := canonicalTempDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o600))
			p := domain.RenamePreview{DirectoryID: identity, OriginalName: "a", NewName: "b", OriginalPath: filepath.Join(dir, "a"), NewPath: filepath.Join(dir, "b")}
			result := NewRenamerService(&osfs.OSFileSystem{}).ExecuteRename([]domain.RenamePreview{p})
			require.False(t, result.Success)
			require.FileExists(t, p.OriginalPath)
			require.NoFileExists(t, p.NewPath)
		})
	}
}
