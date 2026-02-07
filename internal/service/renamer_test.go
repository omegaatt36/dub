package service

import (
	"fmt"
	"testing"

	"github.com/omegaatt36/dub/internal/domain"
)

func TestRenamerService_PreviewRename(t *testing.T) {
	mockFS := &MockFileSystem{
		RenameFunc: func(old, new string) error { return nil },
	}
	svc := NewRenamerService(mockFS)

	t.Run("generates previews with extensions", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "old1.txt", Path: "/dir/old1.txt", Extension: ".txt"},
			{Name: "old2.txt", Path: "/dir/old2.txt", Extension: ".txt"},
		}
		names := []string{"new1", "new2"}

		previews, err := svc.PreviewRename(files, names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(previews) != 2 {
			t.Fatalf("got %d previews, want 2", len(previews))
		}
		if previews[0].NewName != "new1.txt" {
			t.Errorf("got new name %q, want %q", previews[0].NewName, "new1.txt")
		}
		if previews[1].NewName != "new2.txt" {
			t.Errorf("got new name %q, want %q", previews[1].NewName, "new2.txt")
		}
	})

	t.Run("preserves extension if already present", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "old.txt", Path: "/dir/old.txt", Extension: ".txt"},
		}
		names := []string{"new.txt"}

		previews, err := svc.PreviewRename(files, names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if previews[0].NewName != "new.txt" {
			t.Errorf("got %q, want %q (should not double extension)", previews[0].NewName, "new.txt")
		}
	})

	t.Run("detects conflicts (all duplicates marked)", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "a.txt", Path: "/dir/a.txt", Extension: ".txt"},
			{Name: "b.txt", Path: "/dir/b.txt", Extension: ".txt"},
			{Name: "c.txt", Path: "/dir/c.txt", Extension: ".txt"},
		}
		names := []string{"same", "same", "unique"}

		previews, err := svc.PreviewRename(files, names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !previews[0].Conflict {
			t.Error("first duplicate should be marked as conflict")
		}
		if !previews[1].Conflict {
			t.Error("second duplicate should be marked as conflict")
		}
		if previews[2].Conflict {
			t.Error("unique name should not be conflict")
		}
	})

	t.Run("mismatched count returns error", func(t *testing.T) {
		files := []domain.FileItem{{Name: "a.txt"}}
		names := []string{"new1", "new2"}

		_, err := svc.PreviewRename(files, names)
		if err != domain.ErrMismatchedNames {
			t.Errorf("expected ErrMismatchedNames, got: %v", err)
		}
	})

	t.Run("empty new name uses original", func(t *testing.T) {
		files := []domain.FileItem{
			{Name: "keep.txt", Path: "/dir/keep.txt", Extension: ".txt"},
		}
		names := []string{""}

		previews, err := svc.PreviewRename(files, names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if previews[0].NewName != "keep.txt" {
			t.Errorf("empty name should keep original, got %q", previews[0].NewName)
		}
	})
}

func TestRenamerService_ExecuteRename(t *testing.T) {
	t.Run("renames non-conflict files", func(t *testing.T) {
		var renamedPairs [][]string
		mockFS := &MockFileSystem{
			RenameFunc: func(old, new string) error {
				renamedPairs = append(renamedPairs, []string{old, new})
				return nil
			},
		}
		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalPath: "/dir/a.txt", NewPath: "/dir/x.txt", Conflict: false},
			{OriginalPath: "/dir/b.txt", NewPath: "/dir/y.txt", Conflict: true},
			{OriginalPath: "/dir/c.txt", NewPath: "/dir/z.txt", Conflict: false},
		}

		result := svc.ExecuteRename(previews)
		if result.RenamedCount != 2 {
			t.Errorf("got %d renamed, want 2", result.RenamedCount)
		}
		if !result.Success {
			t.Error("expected success")
		}
		if len(renamedPairs) != 2 {
			t.Errorf("rename called %d times, want 2", len(renamedPairs))
		}
	})

	t.Run("skips same-path renames", func(t *testing.T) {
		var called int
		mockFS := &MockFileSystem{
			RenameFunc: func(old, new string) error {
				called++
				return nil
			},
		}
		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalPath: "/dir/same.txt", NewPath: "/dir/same.txt"},
		}

		result := svc.ExecuteRename(previews)
		if called != 0 {
			t.Error("should not call rename for same path")
		}
		if result.RenamedCount != 0 {
			t.Errorf("got %d renamed, want 0", result.RenamedCount)
		}
	})

	t.Run("collects errors", func(t *testing.T) {
		mockFS := &MockFileSystem{
			RenameFunc: func(old, new string) error {
				return fmt.Errorf("permission denied")
			},
		}
		svc := NewRenamerService(mockFS)

		previews := []domain.RenamePreview{
			{OriginalName: "a.txt", OriginalPath: "/dir/a.txt", NewPath: "/dir/x.txt"},
		}

		result := svc.ExecuteRename(previews)
		if result.Success {
			t.Error("should not be successful with errors")
		}
		if len(result.Errors) != 1 {
			t.Errorf("got %d errors, want 1", len(result.Errors))
		}
	})
}
