package domain

// DiffType represents the type of a diff segment.
type DiffType int

const (
	DiffEqual DiffType = iota
	DiffDelete
	DiffInsert
)

// DiffSegment represents a segment of a diff result.
type DiffSegment struct {
	Text string
	Type DiffType
}

// ComputeDiff computes character-level diff between two strings using LCS.
// Returns two segment slices: one for the old string (Equal+Delete)
// and one for the new string (Equal+Insert).
func ComputeDiff(old, new string) ([]DiffSegment, []DiffSegment) {
	if old == new {
		if old == "" {
			return nil, nil
		}
		seg := []DiffSegment{{Text: old, Type: DiffEqual}}
		return seg, seg
	}

	oldRunes := []rune(old)
	newRunes := []rune(new)

	// Compute LCS table
	lcs := computeLCS(oldRunes, newRunes)

	// Backtrack to build diff operations
	type op struct {
		char rune
		kind DiffType
	}
	var ops []op

	i, j := len(oldRunes), len(newRunes)
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldRunes[i-1] == newRunes[j-1] {
			ops = append(ops, op{oldRunes[i-1], DiffEqual})
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			ops = append(ops, op{newRunes[j-1], DiffInsert})
			j--
		} else {
			ops = append(ops, op{oldRunes[i-1], DiffDelete})
			i--
		}
	}

	// Reverse ops (built in reverse order)
	for left, right := 0, len(ops)-1; left < right; left, right = left+1, right-1 {
		ops[left], ops[right] = ops[right], ops[left]
	}

	// Merge consecutive same-type ops into segments
	var oldSegs, newSegs []DiffSegment
	for _, o := range ops {
		switch o.kind {
		case DiffEqual:
			oldSegs = appendSegment(oldSegs, string(o.char), DiffEqual)
			newSegs = appendSegment(newSegs, string(o.char), DiffEqual)
		case DiffDelete:
			oldSegs = appendSegment(oldSegs, string(o.char), DiffDelete)
		case DiffInsert:
			newSegs = appendSegment(newSegs, string(o.char), DiffInsert)
		}
	}

	return oldSegs, newSegs
}

func appendSegment(segs []DiffSegment, char string, typ DiffType) []DiffSegment {
	if len(segs) > 0 && segs[len(segs)-1].Type == typ {
		segs[len(segs)-1].Text += char
		return segs
	}
	return append(segs, DiffSegment{Text: char, Type: typ})
}

func computeLCS(a, b []rune) [][]int {
	m, n := len(a), len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}
	return table
}
