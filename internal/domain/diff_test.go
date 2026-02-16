package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// joinSegments reconstructs the full text from segments of given types.
func joinSegments(segs []DiffSegment, types ...DiffType) string {
	typeSet := make(map[DiffType]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	var b strings.Builder
	for _, s := range segs {
		if typeSet[s.Type] {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

// joinAll reconstructs the full text from all segments.
func joinAll(segs []DiffSegment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestComputeDiff(t *testing.T) {
	t.Run("identical strings", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("hello", "hello")
		assert.Equal(t, []DiffSegment{{Text: "hello", Type: DiffEqual}}, oldSegs)
		assert.Equal(t, []DiffSegment{{Text: "hello", Type: DiffEqual}}, newSegs)
	})

	t.Run("completely different", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("abc", "xyz")
		assert.Equal(t, []DiffSegment{{Text: "abc", Type: DiffDelete}}, oldSegs)
		assert.Equal(t, []DiffSegment{{Text: "xyz", Type: DiffInsert}}, newSegs)
	})

	t.Run("prefix change", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("photo_001", "vacation_001")
		// Verify reconstructed text is correct
		assert.Equal(t, "photo_001", joinAll(oldSegs))
		assert.Equal(t, "vacation_001", joinAll(newSegs))
		// Should have delete segments in old, insert segments in new
		assert.NotEmpty(t, joinSegments(oldSegs, DiffDelete), "old should have deletions")
		assert.NotEmpty(t, joinSegments(newSegs, DiffInsert), "new should have insertions")
		// Equal parts should match between old and new
		assert.Equal(t,
			joinSegments(oldSegs, DiffEqual),
			joinSegments(newSegs, DiffEqual),
			"equal segments should match",
		)
	})

	t.Run("suffix change", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("file_old", "file_new")
		assert.Equal(t, "file_old", joinAll(oldSegs))
		assert.Equal(t, "file_new", joinAll(newSegs))
		// "file_" should be equal in both
		assert.True(t, strings.HasPrefix(joinSegments(oldSegs, DiffEqual), "file_"))
	})

	t.Run("middle change", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("img_cat_01", "img_dog_01")
		assert.Equal(t, "img_cat_01", joinAll(oldSegs))
		assert.Equal(t, "img_dog_01", joinAll(newSegs))
		// "img_" prefix and "_01" suffix should be equal
		eqOld := joinSegments(oldSegs, DiffEqual)
		assert.True(t, strings.HasPrefix(eqOld, "img_"))
		assert.True(t, strings.HasSuffix(eqOld, "_01"))
	})

	t.Run("insertion only", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("ab", "aXb")
		assert.Equal(t, "ab", joinAll(oldSegs))
		assert.Equal(t, "aXb", joinAll(newSegs))
		// No deletions in old
		assert.Empty(t, joinSegments(oldSegs, DiffDelete))
		// "X" inserted in new
		assert.Equal(t, "X", joinSegments(newSegs, DiffInsert))
	})

	t.Run("deletion only", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("aXb", "ab")
		assert.Equal(t, "aXb", joinAll(oldSegs))
		assert.Equal(t, "ab", joinAll(newSegs))
		// "X" deleted from old
		assert.Equal(t, "X", joinSegments(oldSegs, DiffDelete))
		// No insertions in new
		assert.Empty(t, joinSegments(newSegs, DiffInsert))
	})

	t.Run("empty old string", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("", "new")
		assert.Empty(t, oldSegs)
		assert.Equal(t, []DiffSegment{{Text: "new", Type: DiffInsert}}, newSegs)
	})

	t.Run("empty new string", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("old", "")
		assert.Equal(t, []DiffSegment{{Text: "old", Type: DiffDelete}}, oldSegs)
		assert.Empty(t, newSegs)
	})

	t.Run("both empty", func(t *testing.T) {
		oldSegs, newSegs := ComputeDiff("", "")
		assert.Empty(t, oldSegs)
		assert.Empty(t, newSegs)
	})
}
