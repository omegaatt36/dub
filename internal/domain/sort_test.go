package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNaturalSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "numeric ordering",
			input:    []string{"file_10", "file_2", "file_1", "file_20", "file_3"},
			expected: []string{"file_1", "file_2", "file_3", "file_10", "file_20"},
		},
		{
			name:     "mixed alpha and numeric",
			input:    []string{"b2", "a10", "a2", "b1", "a1"},
			expected: []string{"a1", "a2", "a10", "b1", "b2"},
		},
		{
			name:     "case insensitive",
			input:    []string{"B.txt", "a.txt", "C.txt"},
			expected: []string{"a.txt", "B.txt", "C.txt"},
		},
		{
			name:     "purely numeric names",
			input:    []string{"100", "20", "3", "1"},
			expected: []string{"1", "3", "20", "100"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single element",
			input:    []string{"file.txt"},
			expected: []string{"file.txt"},
		},
		{
			name:     "identical names",
			input:    []string{"abc", "abc", "abc"},
			expected: []string{"abc", "abc", "abc"},
		},
		{
			name:     "leading zeros",
			input:    []string{"file_001", "file_01", "file_1"},
			expected: []string{"file_1", "file_01", "file_001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := make([]FileItem, len(tt.input))
			for i, name := range tt.input {
				files[i] = FileItem{Name: name}
			}

			NaturalSort(files)

			for i, f := range files {
				assert.Equal(t, tt.expected[i], f.Name, "position %d", i)
			}
		})
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		size     uint64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatFileSize(tt.size))
		})
	}
}

func TestFileTypeIcon(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".jpg", "image"},
		{".mp4", "video"},
		{".mp3", "audio"},
		{".pdf", "pdf"},
		{".txt", "document"},
		{".csv", "spreadsheet"},
		{".zip", "archive"},
		{".go", "code"},
		{".xyz", "file"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			assert.Equal(t, tt.expected, FileTypeIcon(tt.ext))
		})
	}
}

func TestSortFiles(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	files := func() []FileItem {
		return []FileItem{
			{Name: "b", Size: 30, ModTime: t0.Add(2 * time.Hour)},
			{Name: "a", Size: 30, ModTime: t0},
			{Name: "c", Size: 10, ModTime: t0.Add(time.Hour)},
		}
	}
	names := func(fs []FileItem) []string {
		out := make([]string, len(fs))
		for i, f := range fs {
			out[i] = f.Name
		}
		return out
	}

	tests := []struct {
		order    SortOrder
		expected []string
	}{
		{SortOrder{SortByName, false}, []string{"a", "b", "c"}},
		{SortOrder{SortByName, true}, []string{"c", "b", "a"}},
		{SortOrder{SortByModified, false}, []string{"a", "c", "b"}},
		{SortOrder{SortByModified, true}, []string{"b", "c", "a"}},
		{SortOrder{SortBySize, false}, []string{"c", "a", "b"}},
		// Equal sizes tie-break by natural name order even when descending.
		{SortOrder{SortBySize, true}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.order.String(), func(t *testing.T) {
			fs := files()
			SortFiles(fs, tt.order)
			assert.Equal(t, tt.expected, names(fs))
		})
	}
}

func TestParseSortOrder(t *testing.T) {
	for _, o := range []SortOrder{{SortByName, false}, {SortByModified, true}, {SortBySize, true}} {
		got, err := ParseSortOrder(o.String())
		assert.NoError(t, err)
		assert.Equal(t, o, got)
	}
	for _, bad := range []string{"", "name", "name:up", "color:asc", "name:asc:x"} {
		_, err := ParseSortOrder(bad)
		assert.ErrorIs(t, err, ErrInvalidSortOrder, bad)
	}
}

func TestFileItemIsHidden(t *testing.T) {
	assert.True(t, FileItem{Name: ".env"}.IsHidden())
	assert.True(t, FileItem{Name: "._photo.jpg"}.IsHidden())
	assert.False(t, FileItem{Name: "photo.jpg"}.IsHidden())
	assert.False(t, FileItem{Name: "file.hidden.txt"}.IsHidden())
}

func TestFolderItem_IsHidden(t *testing.T) {
	assert.True(t, FolderItem{Name: ".git"}.IsHidden())
	assert.False(t, FolderItem{Name: "samples"}.IsHidden())
}

func TestSortFolders(t *testing.T) {
	folders := []FolderItem{{Name: "Beta"}, {Name: "part_10"}, {Name: "alpha"}, {Name: "part_2"}}
	SortFolders(folders)

	var got []string
	for _, f := range folders {
		got = append(got, f.Name)
	}
	assert.Equal(t, []string{"alpha", "Beta", "part_2", "part_10"}, got)
}
