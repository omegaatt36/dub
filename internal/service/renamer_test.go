package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/mock"
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

		previews, err := svc.PreviewRename(files, names)
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

		previews, err := svc.PreviewRename(files, names)
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

		previews, err := svc.PreviewRename(files, names)
		require.NoError(t, err)
		assert.True(t, previews[0].Conflict, "first duplicate should be conflict")
		assert.True(t, previews[1].Conflict, "second duplicate should be conflict")
		assert.False(t, previews[2].Conflict, "unique name should not be conflict")
	})

	t.Run("mismatched count returns error", func(t *testing.T) {
		files := []domain.FileItem{{Name: "a.txt"}}
		names := []string{"new1", "new2"}

		_, err := svc.PreviewRename(files, names)
		assert.ErrorIs(t, err, domain.ErrMismatchedNames)
	})

	t.Run("empty new name uses original", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "keep.txt", Path: "/dir/keep.txt", Extension: ".txt"},
		}
		names := []string{""}

		previews, err := svc.PreviewRename(files, names)
		require.NoError(t, err)
		assert.Equal(t, "keep.txt", previews[0].NewName, "empty name should keep original")
	})

	t.Run("rejects path traversal in new names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
		}
		names := []string{"../evil"}

		_, err := svc.PreviewRename(files, names)
		assert.ErrorIs(t, err, domain.ErrInvalidFileName)
	})

	t.Run("rejects slash in new names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
		}
		names := []string{"sub/file"}

		_, err := svc.PreviewRename(files, names)
		assert.ErrorIs(t, err, domain.ErrInvalidFileName)
	})

	t.Run("rejects backslash in new names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
		}
		names := []string{"sub\\file"}

		_, err := svc.PreviewRename(files, names)
		assert.ErrorIs(t, err, domain.ErrInvalidFileName)
	})

	t.Run("computes diff segments for changed names", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "photo_001.txt", Path: "/dir/photo_001.txt", Extension: ".txt"},
		}
		names := []string{"vacation_001"}

		previews, err := svc.PreviewRename(files, names)
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

		previews, err := svc.PreviewRename(files, names)
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
	})
}
