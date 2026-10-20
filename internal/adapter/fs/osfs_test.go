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
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
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
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
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

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
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
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
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

func TestOSFileSystemRenameNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	fs := &OSFileSystem{}
	oldPath, newPath := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(oldPath, []byte("a"), 0o600))
	require.NoError(t, os.Symlink("missing", newPath))
	require.Error(t, fs.Rename(oldPath, newPath))
	require.FileExists(t, oldPath)
	target, err := os.Readlink(newPath)
	require.NoError(t, err)
	require.Equal(t, "missing", target)
}

func TestRenameDirectoryAnchorsSelectedAlias(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	require.NoError(t, os.Mkdir(first, 0o700))
	require.NoError(t, os.Mkdir(second, 0o700))
	for _, dir := range []string{first, second} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte(dir), 0o600))
	}
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(first, alias))
	session, err := (&OSFileSystem{}).OpenRenameDirectory(alias)
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, os.Remove(alias))
	require.NoError(t, os.Symlink(second, alias))
	require.NoError(t, session.Rename("a", "b"))
	require.FileExists(t, filepath.Join(first, "b"))
	require.FileExists(t, filepath.Join(second, "a"))
	require.NoFileExists(t, filepath.Join(second, "b"))
	require.Error(t, session.Rename("../second/a", "c"))
}
