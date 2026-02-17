package service

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/omegaatt36/dub/internal/mock"
	"github.com/omegaatt36/dub/internal/testutil"
)

func TestScannerService_Scan(t *testing.T) {
	t.Run("scans files and sorts naturally", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		mockFS.EXPECT().ReadDir("/test").Return([]os.DirEntry{
			testutil.NewMockDirEntry("file_10.txt", 100),
			testutil.NewMockDirEntry("file_2.txt", 200),
			testutil.NewMockDirEntry("file_1.txt", 300),
			testutil.NewMockDirDirEntry("subdir"),
		}, nil)

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/test")
		require.NoError(t, err)
		require.Len(t, files, 3, "directories excluded")

		expected := []string{"file_1.txt", "file_2.txt", "file_10.txt"}
		for i, f := range files {
			assert.Equal(t, expected[i], f.Name, "position %d", i)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		mockFS.EXPECT().ReadDir("/empty").Return([]os.DirEntry{}, nil)

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/empty")
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("extracts extension", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		mockFS.EXPECT().ReadDir("/test").Return([]os.DirEntry{
			testutil.NewMockDirEntry("photo.JPG", 1000),
		}, nil)

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/test")
		require.NoError(t, err)

		assert.Equal(t, ".jpg", files[0].Extension)
	})

	t.Run("populates ModTime from file info", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockFS := mock.NewMockFileSystem(ctrl)

		fixedTime := time.Date(2026, 2, 17, 10, 30, 0, 0, time.UTC)
		mockFS.EXPECT().ReadDir("/test").Return([]os.DirEntry{
			testutil.NewMockDirEntryWithModTime("photo.jpg", 500, fixedTime),
		}, nil)

		scanner := NewScannerService(mockFS)
		files, err := scanner.Scan("/test")
		require.NoError(t, err)
		require.Len(t, files, 1)
		assert.Equal(t, fixedTime, files[0].ModTime)
	})
}
