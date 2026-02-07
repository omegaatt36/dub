package domain

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// NaturalSort sorts a slice of FileItems by name using natural sort order.
// Numeric sequences within names are compared as integers, so
// "file_2" < "file_10" instead of lexicographic "file_10" < "file_2".
func NaturalSort(files []FileItem) {
	slices.SortFunc(files, func(a, b FileItem) int {
		return naturalCompare(a.Name, b.Name)
	})
}

// naturalCompare compares two strings using natural sort ordering.
// Returns -1 if a < b, 1 if a > b, and 0 if a == b.
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

// extractNumber extracts a numeric value starting at position start in s.
// Returns the numeric value and the end position.
func extractNumber(s string, start int) (uint64, int) {
	var num uint64
	i := start
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		num = num*10 + uint64(s[i]-'0')
		i++
	}
	return num, i
}
