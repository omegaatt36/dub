package service

import (
	"strings"
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
		mockPM.EXPECT().Compile("file_").Return(func(name string) bool {
			return strings.HasPrefix(name, "file_")
		}, nil).Times(1)

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
		var stems []string
		mockPM.EXPECT().Compile("test").Return(func(name string) bool {
			stems = append(stems, name)
			return true
		}, nil)

		svc := NewPatternService(mockPM)
		_, err := svc.MatchFiles(testFiles, "test")
		require.NoError(t, err)
		assert.Equal(t, []string{"hello", "55688"}, stems)
	})

	t.Run("expands shortcuts before matching", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		mockPM.EXPECT().ExpandShortcuts("[serial]").Return("expanded_[serial]")
		mockPM.EXPECT().Compile("expanded_[serial]").Return(func(string) bool { return true }, nil)

		svc := NewPatternService(mockPM)
		_, err := svc.MatchFiles(files, "[serial]")
		require.NoError(t, err)
	})

	t.Run("returns error on compile failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)

		mockPM.EXPECT().ExpandShortcuts("bad_pattern").Return("bad_pattern")
		mockPM.EXPECT().Compile("bad_pattern").Return(nil, domain.ErrInvalidPattern)

		svc := NewPatternService(mockPM)
		_, err := svc.MatchFiles(files, "bad_pattern")
		require.Error(t, err)
	})

	t.Run("validates pattern even without files", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPM := mock.NewMockPatternMatcher(ctrl)
		mockPM.EXPECT().ExpandShortcuts("[").Return("[")
		mockPM.EXPECT().Compile("[").Return(nil, domain.ErrInvalidPattern)

		_, err := NewPatternService(mockPM).MatchFiles(nil, "[")
		require.ErrorIs(t, err, domain.ErrInvalidPattern)
	})
}
