package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/omegaatt36/dub/internal/domain"
)

func TestOSFileSystem_ReadDir(t *testing.T) {
	fs := &OSFileSystem{}

	t.Run("valid directory", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
		os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0644)

		entries, err := fs.ReadDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("got %d entries, want 2", len(entries))
		}
	})

	t.Run("invalid directory", func(t *testing.T) {
		_, err := fs.ReadDir("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for nonexistent path")
		}
		if !errors.Is(err, domain.ErrInvalidPath) {
			t.Errorf("expected ErrInvalidPath, got: %v", err)
		}
	})
}

func TestOSFileSystem_Stat(t *testing.T) {
	fs := &OSFileSystem{}

	t.Run("existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		os.WriteFile(path, []byte("content"), 0644)

		info, err := fs.Stat(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Name() != "test.txt" {
			t.Errorf("got name %q, want %q", info.Name(), "test.txt")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := fs.Stat("/nonexistent/file.txt")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
		if !errors.Is(err, domain.ErrInvalidPath) {
			t.Errorf("expected ErrInvalidPath, got: %v", err)
		}
	})
}

func TestOSFileSystem_Rename(t *testing.T) {
	fs := &OSFileSystem{}

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	os.WriteFile(oldPath, []byte("content"), 0644)

	err := fs.Rename(oldPath, newPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(newPath); err != nil {
		t.Error("new file should exist after rename")
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old file should not exist after rename")
	}
}
