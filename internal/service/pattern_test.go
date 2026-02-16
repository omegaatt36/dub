package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/mock"
)

func TestPatternService_MatchFiles(t *testing.T) {
	files := []domain.FileItem{
		{Name: "file_001.txt", Extension: ".txt"},
		{Name: "file_002.txt", Extension: ".txt"},
		{Name: "photo_001.jpg", Extension: ".jpg"},
		{Name: "document.pdf", Extension: ".pdf"},
	}

	t.Run("empty pattern returns all", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		svc := NewPatternService(mockPM)
		result, err := svc.MatchFiles(files, "")
		require.NoError(t, err)
		assert.Len(t, result, len(files))
	})

	t.Run("filters by pattern", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		mockPM.EXPECT().ExpandShortcuts("file_").Return("file_")
		mockPM.EXPECT().Match("file_", "file_001").Return(true, nil)
		mockPM.EXPECT().Match("file_", "file_002").Return(true, nil)
		mockPM.EXPECT().Match("file_", "photo_001").Return(false, nil)
		mockPM.EXPECT().Match("file_", "document").Return(false, nil)

		svc := NewPatternService(mockPM)
		result, err := svc.MatchFiles(files, "file_")
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("matches against stem not full filename", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		testFiles := []domain.FileItem{
			{Name: "hello.txt", Extension: ".txt"},
			{Name: "55688.pdf", Extension: ".pdf"},
		}

		mockPM.EXPECT().ExpandShortcuts("test").Return("test")
		mockPM.EXPECT().Match("test", "hello").Return(true, nil)
		mockPM.EXPECT().Match("test", "55688").Return(true, nil)

		svc := NewPatternService(mockPM)
		_, _ = svc.MatchFiles(testFiles, "test")
	})

	t.Run("expands shortcuts before matching", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		mockPM.EXPECT().ExpandShortcuts("[serial]").Return("expanded_[serial]")
		mockPM.EXPECT().Match("expanded_[serial]", gomock.Any()).Return(true, nil).Times(4)

		svc := NewPatternService(mockPM)
		_, err := svc.MatchFiles(files, "[serial]")
		require.NoError(t, err)
	})

	t.Run("returns error on match failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		mockPM.EXPECT().ExpandShortcuts("bad_pattern").Return("bad_pattern")
		mockPM.EXPECT().Match("bad_pattern", gomock.Any()).Return(false, domain.ErrInvalidPattern)

		svc := NewPatternService(mockPM)
		_, err := svc.MatchFiles(files, "bad_pattern")
		require.Error(t, err)
	})
}
