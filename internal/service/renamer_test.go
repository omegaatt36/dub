package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/mock"
	"github.com/omegaatt36/dub/internal/port"
)

func TestRenamerService_PreviewRename(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mock.NewMockFileSystem(ctrl)
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

func TestRenamerService_ExecuteRename(t *testing.T) {
	t.Run("renames non-conflict files", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		mockFS.EXPECT().Rename("/dir/a.txt", "/dir/x.txt").Return(nil)
		mockFS.EXPECT().Rename("/dir/c.txt", "/dir/z.txt").Return(nil)

		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalPath: "/dir/a.txt", NewPath: "/dir/x.txt", Conflict: false},
			{OriginalPath: "/dir/b.txt", NewPath: "/dir/y.txt", Conflict: true},
			{OriginalPath: "/dir/c.txt", NewPath: "/dir/z.txt", Conflict: false},
		}

		result := svc.ExecuteRename(previews)
		assert.Equal(t, 2, result.RenamedCount)
		assert.True(t, result.Success)
	})

	t.Run("skips same-path renames", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalPath: "/dir/same.txt", NewPath: "/dir/same.txt"},
		}

		result := svc.ExecuteRename(previews)
		assert.Equal(t, 0, result.RenamedCount)
	})

	t.Run("collects errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		mockFS.EXPECT().Rename("/dir/a.txt", "/dir/x.txt").Return(fmt.Errorf("permission denied"))

		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalName: "a.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/x.txt"},
		}

		result := svc.ExecuteRename(previews)
		assert.False(t, result.Success)
		assert.Len(t, result.Errors, 1)
		assert.True(t, result.RolledBack)
		assert.Equal(t, 0, result.RenamedCount)
	})

	t.Run("rolls back completed renames on error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		// First rename succeeds
		mockFS.EXPECT().Rename("/dir/a.txt", "/dir/x.txt").Return(nil)
		// Second rename fails
		mockFS.EXPECT().Rename("/dir/b.txt", "/dir/y.txt").Return(fmt.Errorf("permission denied"))
		// Rollback: reverse first rename
		mockFS.EXPECT().Rename("/dir/x.txt", "/dir/a.txt").Return(nil)

		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalName: "a.txt", OriginalPath: "/dir/a.txt", NewName: "x.txt", NewPath: "/dir/x.txt"},
			{OriginalName: "b.txt", OriginalPath: "/dir/b.txt", NewName: "y.txt", NewPath: "/dir/y.txt"},
		}

		result := svc.ExecuteRename(previews)
		assert.False(t, result.Success)
		assert.True(t, result.RolledBack)
		assert.Equal(t, 0, result.RenamedCount)
		assert.Empty(t, result.RollbackErrors)
	})

	t.Run("reports rollback errors when rollback fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		// First rename succeeds
		mockFS.EXPECT().Rename("/dir/a.txt", "/dir/x.txt").Return(nil)
		// Second rename fails
		mockFS.EXPECT().Rename("/dir/b.txt", "/dir/y.txt").Return(fmt.Errorf("disk full"))
		// Rollback fails too
		mockFS.EXPECT().Rename("/dir/x.txt", "/dir/a.txt").Return(fmt.Errorf("disk full"))

		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalName: "a.txt", OriginalPath: "/dir/a.txt", NewName: "x.txt", NewPath: "/dir/x.txt"},
			{OriginalName: "b.txt", OriginalPath: "/dir/b.txt", NewName: "y.txt", NewPath: "/dir/y.txt"},
		}

		result := svc.ExecuteRename(previews)
		assert.False(t, result.Success)
		assert.True(t, result.RolledBack)
		assert.Len(t, result.RollbackErrors, 1)
	})

	t.Run("no rollback when all renames succeed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		mockFS.EXPECT().Rename("/dir/a.txt", "/dir/x.txt").Return(nil)
		mockFS.EXPECT().Rename("/dir/b.txt", "/dir/y.txt").Return(nil)

		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalName: "a.txt", OriginalPath: "/dir/a.txt", NewName: "x.txt", NewPath: "/dir/x.txt"},
			{OriginalName: "b.txt", OriginalPath: "/dir/b.txt", NewName: "y.txt", NewPath: "/dir/y.txt"},
		}

		result := svc.ExecuteRename(previews)
		assert.True(t, result.Success)
		assert.False(t, result.RolledBack)
		assert.Empty(t, result.RollbackErrors)
	})
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
