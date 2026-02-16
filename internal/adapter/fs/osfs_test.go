package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/domain"
)

func TestOSFileSystem_ReadDir(t *testing.T) {
	fs := &OSFileSystem{}

	t.Run("valid directory", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644)

		entries, err := fs.ReadDir(dir)
		require.NoError(t, err)
		assert.Len(t, entries, 2)
	})

	t.Run("invalid directory", func(t *testing.T) {
		_, err := fs.ReadDir("/nonexistent/path")
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrInvalidPath)
	})
}

func TestOSFileSystem_Stat(t *testing.T) {
	fs := &OSFileSystem{}

	t.Run("existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		_ = os.WriteFile(path, []byte("content"), 0o644)

		info, err := fs.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, "test.txt", info.Name())
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := fs.Stat("/nonexistent/file.txt")
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrInvalidPath)
	})
}

func TestOSFileSystem_Rename(t *testing.T) {
	fs := &OSFileSystem{}

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	_ = os.WriteFile(oldPath, []byte("content"), 0o644)

	require.NoError(t, fs.Rename(oldPath, newPath))

	assert.FileExists(t, newPath)
	assert.NoFileExists(t, oldPath)
}

func TestOSFileSystem_ReadFile(t *testing.T) {
	fs := &OSFileSystem{}

	t.Run("reads file content", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		_ = os.WriteFile(path, []byte("hello\nworld"), 0o644)

		content, err := fs.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello\nworld"), content)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, err := fs.ReadFile("/nonexistent/file.txt")
		require.Error(t, err)
	})
}
