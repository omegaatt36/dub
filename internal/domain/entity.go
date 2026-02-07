package domain

import "fmt"

// FileItem represents a file in the selected directory.
type FileItem struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Extension string `json:"extension"`
	Size      uint64 `json:"size"`
}

// RenamePreview represents a single file rename preview.
type RenamePreview struct {
	OriginalName string `json:"originalName"`
	NewName      string `json:"newName"`
	OriginalPath string `json:"originalPath"`
	NewPath      string `json:"newPath"`
	Conflict     bool   `json:"conflict"`
}

// RenameResult represents the outcome of a batch rename operation.
type RenameResult struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	RenamedCount int      `json:"renamedCount"`
	Errors       []string `json:"errors"`
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
