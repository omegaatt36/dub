package domain

import "strings"

// IsRenameInternal reserves the journal and staging namespace. These entries
// must never become user-selected sources, even when hidden files are visible.
func IsRenameInternal(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), ".dub-rename-")
}
