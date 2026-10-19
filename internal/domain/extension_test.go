package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtension(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"photo.jpg", ".jpg"},
		{"photo.JPG", ".JPG"}, // case is kept so TrimSuffix(name, ext) is exact
		{"README", ""},
		{"file.hidden.txt", ".txt"},
		{"IMG.2024.01.jpg", ".jpg"},
		{".env", ""},
		{".bashrc.bak", ".bak"},
		{"xxx.tar.gz", ".tar.gz"},
		{"xxx.TAR.GZ", ".TAR.GZ"},
		{"xxx.tar.bz2", ".tar.bz2"},
		{"xxx.tar.xz", ".tar.xz"},
		{"xxx.tar.zst", ".tar.zst"},
		{"backup.2024.tar.gz", ".tar.gz"},
		{"xxx.tar", ".tar"},
		{"xxx.gz", ".gz"},
		{"tar.gz", ".gz"},  // "tar" here is the name, not an extension
		{".tar.gz", ".gz"}, // a dotfile whose name happens to look like one
		{"my.tarball.gz", ".gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Extension(tt.name))
		})
	}
}

// The stem is computed as TrimSuffix(name, ext) throughout the app, so the
// extension must be an exact suffix of the name or the stem keeps a stray tail.
func TestExtensionIsAnExactSuffixOfTheName(t *testing.T) {
	tests := []struct{ name, stem string }{
		{"xxx.tar.gz", "xxx"},
		{"IMG_01.JPG", "IMG_01"},
		{".env", ".env"},
		{"README", "README"},
		{"a.b.c", "a.b"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.stem, strings.TrimSuffix(tt.name, Extension(tt.name)), tt.name)
	}
}

func TestFileTypeIconIgnoresCaseAndSeesTarballs(t *testing.T) {
	assert.Equal(t, "image", FileTypeIcon(".JPG"))
	assert.Equal(t, "archive", FileTypeIcon(".tar.gz"))
	assert.Equal(t, "archive", FileTypeIcon(".TAR.XZ"))
	assert.Equal(t, "file", FileTypeIcon(""))
}

func TestTemplateOnMultiDotAndUppercaseNames(t *testing.T) {
	tar := FileItem{Name: "xxx.tar.gz", Extension: Extension("xxx.tar.gz")}
	assert.Equal(t, "xxx", ExpandTemplate("{original}", tar, 0))
	assert.Equal(t, "tar.gz", ExpandTemplate("{ext}", tar, 0))

	jpg := FileItem{Name: "IMG_01.JPG", Extension: Extension("IMG_01.JPG")}
	assert.Equal(t, "IMG_01", ExpandTemplate("{original}", jpg, 0), "an uppercase extension is not part of the stem")
	assert.Equal(t, "jpg", ExpandTemplate("{ext|lower}", jpg, 0))

	names, err := FindReplace([]FileItem{jpg, tar}, `\d+$|xxx$`, "N")
	assert.NoError(t, err)
	assert.Equal(t, []string{"IMG_N", "N"}, names, "find/replace anchors on the stem, not the extension")
}
