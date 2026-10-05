package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/service"
)

// dropTestApp builds an App whose filesystem serves one names file, so a drop
// can be followed all the way to the state it leaves behind.
func dropTestApp() *App {
	mfs := &mockFS{
		ReadFileFunc: func(path string) ([]byte, error) {
			if path == "/tmp/names.txt" || path == "/tmp/names.csv" || path == "/tmp/NAMES.TXT" {
				return []byte("first.txt\n\nsecond.txt\n"), nil
			}
			return nil, os.ErrNotExist
		},
		ReadDirFunc: func(path string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "photo.jpg", info: &mockFileInfo{name: "photo.jpg", size: 10}},
			}, nil
		},
		StatFunc: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}
	return NewApp(
		mfs,
		service.NewScannerService(mfs),
		service.NewPatternService(&mockPM{}),
		service.NewRenamerService(mfs),
	)
}

// TestDropFilesScansOutsideTheNamesZone pins the routing the drop event does on
// its own: anywhere but the names editor means "scan this path".
func TestDropFilesScansOutsideTheNamesZone(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		dropTarget  string
		expectedDir string
	}{
		{"scan zone", "/tmp/photos", DropTargetScan, "/tmp/photos"},
		{"no zone reported", "/tmp/photos", "", "/tmp/photos"},
		{"names file outside the names zone", "/tmp/names.txt", DropTargetScan, "/tmp/names.txt"},
		{"names zone but not a name list", "/tmp/photos", DropTargetNames, "/tmp/photos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := dropTestApp()

			app.DropFiles([]string{tt.path}, tt.dropTarget)

			app.mu.Lock()
			defer app.mu.Unlock()
			assert.Equal(t, tt.expectedDir, app.state.SelectedDirectory)
			assert.Len(t, app.state.AllFiles, 1)
		})
	}
}

// TestDropFilesLoadsNamesInsideTheNamesZone is the other half: a .txt or .csv on
// the names editor becomes the manual name list instead of a directory to scan.
func TestDropFilesLoadsNamesInsideTheNamesZone(t *testing.T) {
	for _, path := range []string{"/tmp/names.txt", "/tmp/names.csv", "/tmp/NAMES.TXT"} {
		t.Run(path, func(t *testing.T) {
			app := dropTestApp()

			app.DropFiles([]string{path}, DropTargetNames)

			app.mu.Lock()
			defer app.mu.Unlock()
			assert.Equal(t, []string{"first.txt", "second.txt"}, app.state.NewNames)
			assert.Equal(t, "file", app.state.NamingMethod)
			assert.Empty(t, app.state.Error)
		})
	}
}

// TestDropFilesReportsReadFailures keeps a bad drop visible instead of silently
// leaving the user with the previous state.
func TestDropFilesReportsReadFailures(t *testing.T) {
	app := dropTestApp()

	app.DropFiles([]string{"/tmp/missing.txt"}, DropTargetNames)

	app.mu.Lock()
	defer app.mu.Unlock()
	assert.Contains(t, app.state.Error, "Failed to read file")
}

// TestDropFilesIgnoresEmptyDrops covers the webview reporting a drop with no
// files, which must not clear the current directory.
func TestDropFilesIgnoresEmptyDrops(t *testing.T) {
	app := dropTestApp()
	app.state.SelectedDirectory = "/tmp/kept"

	app.DropFiles(nil, DropTargetScan)

	app.mu.Lock()
	defer app.mu.Unlock()
	assert.Equal(t, "/tmp/kept", app.state.SelectedDirectory)
}

// TestOpenDirectoryDialogWithoutPicker verifies the handler degrades to an
// unchanged page instead of panicking when no picker was injected.
func TestOpenDirectoryDialogWithoutPicker(t *testing.T) {
	app := NewApp(nil, nil, nil, nil)

	path, err := app.OpenDirectoryDialog()

	require.Error(t, err)
	assert.Empty(t, path)
}
