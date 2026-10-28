package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	osfs "github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/port"
)

// Unlike the panic fault-injection tests, these stop a separate process without
// unwinding defers. Recovery must use only persisted state and reacquire the
// directory lock after the kernel closes the terminated process's descriptors.
func TestRecoveryAfterProcessKill(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for stop := 1; stop <= 5; stop++ {
		t.Run(strconv.Itoa(stop), func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, name := range []string{"a", "b"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestRenameCrashHelper$")
			cmd.Env = append(os.Environ(), "DUB_RENAME_CRASH_HELPER="+dir, "DUB_RENAME_CRASH_STOP="+strconv.Itoa(stop))
			cmd.Stderr = os.Stderr
			output, err := cmd.StdoutPipe()
			require.NoError(t, err)
			input, err := cmd.StdinPipe()
			require.NoError(t, err)
			defer input.Close()
			require.NoError(t, cmd.Start())
			defer func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}()
			scanner := bufio.NewScanner(output)
			require.True(t, scanner.Scan(), "child stopped before interruption boundary")
			require.Equal(t, "DUB_RENAME_BOUNDARY", scanner.Text())
			require.NoError(t, cmd.Process.Kill())
			require.Error(t, cmd.Wait(), "helper must terminate without normal cleanup")

			svc := NewRenamerService(&osfs.OSFileSystem{})
			pending, err := svc.RecoveryPending(dir)
			require.NoError(t, err)
			require.True(t, pending)
			result := svc.RecoverRename(dir)
			require.True(t, result.Success, result)
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

func TestRenameCrashHelper(t *testing.T) {
	dir := os.Getenv("DUB_RENAME_CRASH_HELPER")
	if dir == "" {
		return
	}
	stop, err := strconv.Atoi(os.Getenv("DUB_RENAME_CRASH_STOP"))
	require.NoError(t, err)
	previews := safetyPreviews(t, dir, []string{"a", "b"}, []string{"b", "a"})
	result := NewRenamerService(&pausedRenameFS{stop: stop}).ExecuteRename(previews)
	t.Fatalf("helper completed instead of reaching interruption boundary: %+v", result)
}

type pausedRenameFS struct {
	osfs.OSFileSystem
	stop int
}

func (f *pausedRenameFS) OpenRenameDirectory(path string) (port.RenameDirectory, error) {
	d, err := f.OSFileSystem.OpenRenameDirectory(path)
	if err != nil {
		return nil, err
	}
	return &pausedRenameDirectory{RenameDirectory: d, stop: f.stop}, nil
}

type pausedRenameDirectory struct {
	port.RenameDirectory
	calls int
	stop  int
}

func pauseForProcessKill() {
	_, _ = fmt.Fprintln(os.Stdout, "DUB_RENAME_BOUNDARY")
	// The parent keeps this pipe open until it kills the helper.
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func (d *pausedRenameDirectory) Rename(from, to string) error {
	err := d.RenameDirectory.Rename(from, to)
	d.calls++
	if err == nil && d.calls == d.stop {
		pauseForProcessKill()
	}
	return err
}

func (d *pausedRenameDirectory) RemoveJournal() error {
	if d.stop == 5 {
		pauseForProcessKill()
	}
	return d.RenameDirectory.RemoveJournal()
}
