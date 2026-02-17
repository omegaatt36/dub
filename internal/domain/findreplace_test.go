package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindReplace(t *testing.T) {
	files := []FileItem{
		{Name: "photo_001.jpg", Extension: ".jpg"},
		{Name: "photo_002.jpg", Extension: ".jpg"},
		{Name: "document.pdf", Extension: ".pdf"},
	}

	t.Run("basic replacement", func(t *testing.T) {
		names, err := FindReplace(files, "photo", "vacation")
		require.NoError(t, err)
		assert.Equal(t, "vacation_001", names[0])
		assert.Equal(t, "vacation_002", names[1])
		assert.Equal(t, "document", names[2])
	})

	t.Run("capture group swap", func(t *testing.T) {
		names, err := FindReplace(files, `(\w+)_(\d+)`, "${2}_${1}")
		require.NoError(t, err)
		assert.Equal(t, "001_photo", names[0])
		assert.Equal(t, "002_photo", names[1])
		assert.Equal(t, "document", names[2])
	})

	t.Run("no match keeps original stem", func(t *testing.T) {
		names, err := FindReplace(files, "xyz", "abc")
		require.NoError(t, err)
		assert.Equal(t, "photo_001", names[0])
		assert.Equal(t, "document", names[2])
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		_, err := FindReplace(files, "[invalid", "replacement")
		assert.Error(t, err)
	})

	t.Run("empty search returns original stems", func(t *testing.T) {
		names, err := FindReplace(files, "", "prefix")
		require.NoError(t, err)
		assert.Equal(t, "photo_001", names[0])
	})
}
