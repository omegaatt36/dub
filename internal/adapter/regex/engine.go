package regex

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/omegaatt36/dub/internal/domain"
)

// Engine implements port.PatternMatcher using Go's regexp package.
type Engine struct{}

// shortcuts maps user-friendly shortcut tokens to regex patterns.
var shortcuts = map[string]string{
	"[serial]": `(\d+)`,
	"[number]": `(\d+)`,
	"[any]":    `(.*)`,
	"[word]":   `(\w+)`,
	"[alpha]":  `([a-zA-Z]+)`,
}

func (e *Engine) ExpandShortcuts(pattern string) string {
	result := pattern
	for shortcut, regex := range shortcuts {
		result = strings.ReplaceAll(result, shortcut, regex)
	}
	return result
}

func (e *Engine) Match(pattern, name string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("%w: %s", domain.ErrInvalidPattern, err)
	}
	return re.MatchString(name), nil
}
