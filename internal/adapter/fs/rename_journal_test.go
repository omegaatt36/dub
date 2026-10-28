//go:build darwin || linux

package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJournalIdentityBoundToOpenedFile(t *testing.T) {
	for _, create := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "create"}[create], func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			session, err := (&OSFileSystem{}).OpenRenameDirectory(dir)
			require.NoError(t, err)
			defer session.Close()
			d := session.(*renameDirectory)
			path := filepath.Join(dir, journalName)
			if !create {
				require.NoError(t, os.WriteFile(path, []byte("owned"), 0o600))
			}
			d.journalOpened = func() {
				require.NoError(t, os.Rename(path, filepath.Join(dir, "saved")))
				require.NoError(t, os.WriteFile(path, []byte("outside"), 0o600))
			}
			if create {
				require.Error(t, d.CreateJournal([]byte("intent")))
			} else {
				_, err := d.ReadJournal()
				require.Error(t, err)
			}
			require.Error(t, d.RemoveJournal())
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), nil, 0o600))
			require.Error(t, d.Rename("a", "b"))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "outside", string(data))
		})
	}
}
