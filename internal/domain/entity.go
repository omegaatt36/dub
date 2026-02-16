package domain

import "fmt"

type FileItem struct {
	Name      string
	Path      string
	Extension string
	Size      uint64
}

type RenamePreview struct {
	OriginalName string
	NewName      string
	OriginalPath string
	NewPath      string
	Conflict     bool
	OriginalDiff []DiffSegment
	NewDiff      []DiffSegment
}

type RenameResult struct {
	Success      bool
	Message      string
	RenamedCount int
	Errors       []string
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
