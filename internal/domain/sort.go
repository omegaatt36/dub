package domain

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
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

func naturalCompare(a, b string) int {
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)

	ai, bi := 0, 0
	for ai < len(aLower) && bi < len(bLower) {
		aIsDigit := unicode.IsDigit(rune(aLower[ai]))
		bIsDigit := unicode.IsDigit(rune(bLower[bi]))

		switch {
		case aIsDigit && bIsDigit:
			// Extract numeric sequences and compare as numbers
			aNum, aEnd := extractNumber(aLower, ai)
			bNum, bEnd := extractNumber(bLower, bi)

			if c := cmp.Compare(aNum, bNum); c != 0 {
				return c
			}
			// Same numeric value: shorter string of digits first (e.g., "01" < "001")
			aLen := aEnd - ai
			bLen := bEnd - bi
			if c := cmp.Compare(aLen, bLen); c != 0 {
				return c
			}
			ai = aEnd
			bi = bEnd
		default:
			if aLower[ai] != bLower[bi] {
				return cmp.Compare(aLower[ai], bLower[bi])
			}
			ai++
			bi++
		}
	}
	return cmp.Compare(len(aLower), len(bLower))
}

func extractNumber(s string, start int) (uint64, int) {
	var num uint64
	i := start
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		num = num*10 + uint64(s[i]-'0')
		i++
	}
	return num, i
}
