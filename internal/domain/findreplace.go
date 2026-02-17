package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// FindReplace applies a regex search and replace to filename stems.
// Returns new name stems (without extension). Non-matching files keep their original stem.
func FindReplace(files []FileItem, search, replace string) ([]string, error) {
	if search == "" {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = strings.TrimSuffix(f.Name, f.Extension)
		}
		return names, nil
	}

	re, err := regexp.Compile(search)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPattern, err)
	}

	names := make([]string, len(files))
	for i, f := range files {
		stem := strings.TrimSuffix(f.Name, f.Extension)
		if re.MatchString(stem) {
			names[i] = re.ReplaceAllString(stem, replace)
		} else {
			names[i] = stem
		}
	}

	return names, nil
}
