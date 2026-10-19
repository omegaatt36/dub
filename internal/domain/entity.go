package domain

import (
	"fmt"
	"strings"
	"time"
)

type FileItem struct {
	Name      string
	Path      string
	Extension string
	Size      uint64
	ModTime   time.Time
}

// IsHidden reports whether the file is a dotfile. Hidden-ness is a property of
// the name alone, so it needs no filesystem attributes and behaves the same on
// every platform Dub ships on.
func (f FileItem) IsHidden() bool {
	return strings.HasPrefix(f.Name, ".")
}

// FolderItem is a subdirectory as the folder tree lists it.
type FolderItem struct {
	Name string
	Path string
}

// IsHidden reports whether the folder is a dot-folder, by the same name-only
// rule FileItem uses.
func (f FolderItem) IsHidden() bool {
	return strings.HasPrefix(f.Name, ".")
}

type RenamePreview struct {
	// Extension and Size describe the source file. A preview carries them
	// because previews only cover ticked files, so the UI cannot recover them
	// by indexing the (larger) visible list.
	Extension    string
	Size         uint64
	OriginalName string
	NewName      string
	OriginalPath string
	NewPath      string
	Conflict     bool
	OriginalDiff []DiffSegment
	NewDiff      []DiffSegment
}

type RenameResult struct {
	Success        bool
	Message        string
	RenamedCount   int
	Errors         []string
	RolledBack     bool
	RollbackErrors []string
}

// FormatFileSize formats a file size in bytes to a human-readable string.
func FormatFileSize(size uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// FileTypeIcon returns an icon string based on file extension.
func FileTypeIcon(ext string) string {
	ext = strings.ToLower(ext)
	if strings.HasPrefix(ext, tarSuffix+".") {
		return "archive"
	}
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp", ".ico":
		return "image"
	case ".mp4", ".avi", ".mov", ".mkv", ".wmv", ".flv", ".webm":
		return "video"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma":
		return "audio"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".txt", ".rtf", ".odt", ".md":
		return "document"
	case ".xls", ".xlsx", ".csv", ".ods":
		return "spreadsheet"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return "archive"
	case ".go", ".js", ".ts", ".py", ".rs", ".java", ".c", ".cpp", ".h":
		return "code"
	default:
		return "file"
	}
}
