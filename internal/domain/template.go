package domain

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// templateTokenRe matches {name}, {name:format}, {name|pipe}, or {name:format|pipe}.
var templateTokenRe = regexp.MustCompile(`\{(\w+)(?::([^}|]+))?(?:\|(\w+))?\}`)

// ExpandTemplate replaces template tokens in tmpl using data from file and index.
// index is 0-based internally; displayed as 1-based.
func ExpandTemplate(tmpl string, file FileItem, index int) string {
	return templateTokenRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		groups := templateTokenRe.FindStringSubmatch(match)
		name := groups[1]
		format := groups[2]
		pipe := groups[3]

		var value string
		isString := false

		switch name {
		case "index":
			idx := index + 1
			if format != "" {
				width, err := strconv.Atoi(format)
				if err == nil && width > 0 {
					value = fmt.Sprintf("%0*d", width, idx)
				} else {
					value = fmt.Sprintf("%d", idx)
				}
			} else {
				value = fmt.Sprintf("%d", idx)
			}
		case "original":
			value = strings.TrimSuffix(file.Name, file.Extension)
			isString = true
		case "ext":
			value = strings.TrimPrefix(file.Extension, ".")
			isString = true
		case "date":
			if format != "" {
				value = file.ModTime.Format(format)
			} else {
				value = file.ModTime.Format("2006-01-02")
			}
		case "parent":
			value = filepath.Base(filepath.Dir(file.Path))
			isString = true
		default:
			return match // unknown token, leave as-is
		}

		if isString && pipe != "" {
			value = applyPipe(value, pipe)
		}

		return value
	})
}

func applyPipe(s, pipe string) string {
	switch pipe {
	case "upper":
		return strings.ToUpper(s)
	case "lower":
		return strings.ToLower(s)
	case "title":
		return cases.Title(language.English).String(s)
	default:
		return s
	}
}
