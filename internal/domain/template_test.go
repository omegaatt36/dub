package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExpandTemplate(t *testing.T) {
	file := FileItem{
		Name:      "photo_sunset.jpg",
		Path:      "/vacation_photos/photo_sunset.jpg",
		Extension: ".jpg",
		Size:      1024,
		ModTime:   time.Date(2026, 2, 17, 10, 30, 0, 0, time.UTC),
	}

	t.Run("basic index and original", func(t *testing.T) {
		result := ExpandTemplate("{original}_{index}", file, 0)
		assert.Equal(t, "photo_sunset_1", result)
	})

	t.Run("zero-padded index", func(t *testing.T) {
		result := ExpandTemplate("file_{index:3}", file, 4)
		assert.Equal(t, "file_005", result)
	})

	t.Run("ext variable", func(t *testing.T) {
		result := ExpandTemplate("{original}.{ext}", file, 0)
		assert.Equal(t, "photo_sunset.jpg", result)
	})

	t.Run("date default format", func(t *testing.T) {
		result := ExpandTemplate("{date}_{original}", file, 0)
		assert.Equal(t, "2026-02-17_photo_sunset", result)
	})

	t.Run("date custom format", func(t *testing.T) {
		result := ExpandTemplate("{date:20060102}", file, 0)
		assert.Equal(t, "20260217", result)
	})

	t.Run("parent variable", func(t *testing.T) {
		result := ExpandTemplate("{parent}_{index}", file, 0)
		assert.Equal(t, "vacation_photos_1", result)
	})

	t.Run("upper pipe", func(t *testing.T) {
		result := ExpandTemplate("{original|upper}", file, 0)
		assert.Equal(t, "PHOTO_SUNSET", result)
	})

	t.Run("lower pipe", func(t *testing.T) {
		result := ExpandTemplate("{original|lower}", file, 0)
		assert.Equal(t, "photo_sunset", result)
	})

	t.Run("title pipe", func(t *testing.T) {
		result := ExpandTemplate("{original|title}", file, 0)
		assert.Equal(t, "Photo_sunset", result)
	})

	t.Run("parent with upper pipe", func(t *testing.T) {
		result := ExpandTemplate("{parent|upper}", file, 0)
		assert.Equal(t, "VACATION_PHOTOS", result)
	})

	t.Run("pipe ignored on index", func(t *testing.T) {
		result := ExpandTemplate("{index|upper}", file, 0)
		assert.Equal(t, "1", result)
	})

	t.Run("no template tokens passes through", func(t *testing.T) {
		result := ExpandTemplate("plain_name", file, 0)
		assert.Equal(t, "plain_name", result)
	})

	t.Run("mixed tokens and literal text", func(t *testing.T) {
		result := ExpandTemplate("IMG_{date:20060102}_{index:4}", file, 41)
		assert.Equal(t, "IMG_20260217_0042", result)
	})
}
