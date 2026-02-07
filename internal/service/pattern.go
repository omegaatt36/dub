package service

import (
	"strings"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// PatternService handles file filtering by pattern.
type PatternService struct {
	pm port.PatternMatcher
}

func NewPatternService(pm port.PatternMatcher) *PatternService {
	return &PatternService{pm: pm}
}

// MatchFiles filters files by pattern. Empty pattern returns all files.
func (s *PatternService) MatchFiles(files []domain.FileItem, pattern string) ([]domain.FileItem, error) {
	if pattern == "" {
		return files, nil
	}

	expanded := s.pm.ExpandShortcuts(pattern)

	var matched []domain.FileItem
	for _, f := range files {
		// Match against filename stem (without extension) so shortcuts
		// like [alpha] don't accidentally match the extension part.
		stem := strings.TrimSuffix(f.Name, f.Extension)
		ok, err := s.pm.Match(expanded, stem)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, f)
		}
	}
	return matched, nil
}
