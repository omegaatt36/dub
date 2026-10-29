package domain

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SortField is the file attribute a list is ordered by.
type SortField string

const (
	SortByName     SortField = "name"
	SortByModified SortField = "modified"
	SortBySize     SortField = "size"
)

// SortOrder is a field plus a direction. Its string form ("name:asc") is what
// the UI sends, so ParseSortOrder and String are exact inverses.
type SortOrder struct {
	Field SortField
	Desc  bool
}

func (o SortOrder) String() string {
	if o.Desc {
		return string(o.Field) + ":desc"
	}
	return string(o.Field) + ":asc"
}

// ParseSortOrder parses the "field:direction" form produced by String.
func ParseSortOrder(s string) (SortOrder, error) {
	field, dir, ok := strings.Cut(s, ":")
	f := SortField(field)
	if !ok || (f != SortByName && f != SortByModified && f != SortBySize) || (dir != "asc" && dir != "desc") {
		return SortOrder{}, fmt.Errorf("%w: %q", ErrInvalidSortOrder, s)
	}
	return SortOrder{Field: f, Desc: dir == "desc"}, nil
}

// SortFiles orders files in place. Ties fall back to natural name order in both
// directions, so equal sizes or timestamps keep a predictable sequence.
func SortFiles(files []FileItem, order SortOrder) {
	slices.SortStableFunc(files, func(a, b FileItem) int {
		var c int
		switch order.Field {
		case SortByModified:
			c = a.ModTime.Compare(b.ModTime)
		case SortBySize:
			c = cmp.Compare(a.Size, b.Size)
		default:
			c = naturalCompare(a.Name, b.Name)
		}
		if order.Desc {
			c = -c
		}
		if c != 0 {
			return c
		}
		return naturalCompare(a.Name, b.Name)
	})
}

// NaturalSort sorts a slice of FileItems by name using natural sort order.
// Numeric sequences within names are compared as integers, so
// "file_2" < "file_10" instead of lexicographic "file_10" < "file_2".
func NaturalSort(files []FileItem) {
	slices.SortFunc(files, func(a, b FileItem) int {
		return naturalCompare(a.Name, b.Name)
	})
}

// SortFolders orders folders in place by natural name order.
func SortFolders(folders []FolderItem) {
	slices.SortFunc(folders, func(a, b FolderItem) int {
		return naturalCompare(a.Name, b.Name)
	})
}

func naturalCompare(a, b string) int {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		aIsDigit := isASCIIDigit(a[ai])
		bIsDigit := isASCIIDigit(b[bi])

		switch {
		case aIsDigit && bIsDigit:
			aDigits, aEnd := extractDigits(a, ai)
			bDigits, bEnd := extractDigits(b, bi)

			if c := compareNumericDigits(aDigits, bDigits); c != 0 {
				return c
			}
			ai = aEnd
			bi = bEnd
		default:
			ar, aSize := utf8.DecodeRuneInString(a[ai:])
			br, bSize := utf8.DecodeRuneInString(b[bi:])
			al := unicode.ToLower(ar)
			bl := unicode.ToLower(br)
			if al != bl {
				return cmp.Compare(al, bl)
			}
			ai += aSize
			bi += bSize
		}
	}
	return cmp.Compare(len(a)-ai, len(b)-bi)
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func extractDigits(s string, start int) (string, int) {
	i := start
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	return s[start:i], i
}

func compareNumericDigits(a, b string) int {
	aTrim := strings.TrimLeft(a, "0")
	bTrim := strings.TrimLeft(b, "0")
	if c := cmp.Compare(len(aTrim), len(bTrim)); c != 0 {
		return c
	}
	if c := cmp.Compare(aTrim, bTrim); c != 0 {
		return c
	}
	// Same numeric value: shorter string of digits first (e.g., "01" < "001")
	return cmp.Compare(len(a), len(b))
}
