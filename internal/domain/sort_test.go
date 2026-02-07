package domain

import (
	"testing"
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
				if f.Name != tt.expected[i] {
					t.Errorf("position %d: got %q, want %q", i, f.Name, tt.expected[i])
				}
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
			result := FormatFileSize(tt.size)
			if result != tt.expected {
				t.Errorf("FormatFileSize(%d) = %q, want %q", tt.size, result, tt.expected)
			}
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
			result := FileTypeIcon(tt.ext)
			if result != tt.expected {
				t.Errorf("FileTypeIcon(%q) = %q, want %q", tt.ext, result, tt.expected)
			}
		})
	}
}
