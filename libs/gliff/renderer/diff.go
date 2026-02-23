package renderer

import (
	"strconv"
	"strings"
)

// ScrollOp represents a scroll operation.
type ScrollOp struct {
	Direction int // Positive = scroll up (content moves up), Negative = scroll down
	Amount    int // Number of lines
	Top       int // Top of scroll region (0-indexed)
	Bottom    int // Bottom of scroll region (0-indexed, inclusive)
}

// detectScroll analyzes old and new buffers to find scroll operations.
// This implements the Heckel algorithm as used by ncurses:
//
// 1. Find lines that are unique in both old and new (appear exactly once)
// 2. These unique matching lines are anchor points
// 3. Expand from anchors to find contiguous blocks of moved lines
// 4. Validate hunks: require 3+ lines and reasonable shift distance
//
// Returns nil if no beneficial scroll was detected.
func detectScroll(old, new *Buffer) *ScrollOp {
	if old.Height != new.Height || old.Height < 3 {
		return nil
	}

	height := old.Height

	// Build occurrence tables for hashes
	// Count how many times each hash appears in old and new
	oldCount := make(map[uint64]int)
	newCount := make(map[uint64]int)
	oldIndex := make(map[uint64]int) // Last index where hash appears in old
	newIndex := make(map[uint64]int) // Last index where hash appears in new

	for i := range height {
		oldCount[old.hashes[i]]++
		oldIndex[old.hashes[i]] = i
		newCount[new.hashes[i]]++
		newIndex[new.hashes[i]] = i
	}

	// Find unique lines: lines whose hash appears exactly once in both
	// These are our anchor points (Heckel's key insight)
	// newToOld[i] = j means new line i matches old line j
	newToOld := make([]int, height)
	for i := range newToOld {
		newToOld[i] = -1 // -1 means no match
	}

	for i := range height {
		h := new.hashes[i]
		if newCount[h] == 1 && oldCount[h] == 1 {
			// Unique in both - this is an anchor
			newToOld[i] = oldIndex[h]
		}
	}

	// Expand from anchors: if line i matches old line j,
	// check if i+1 matches j+1 (same hash), and so on
	for i := range height {
		if newToOld[i] >= 0 {
			// Expand forward
			for ni, oi := i+1, newToOld[i]+1; ni < height && oi < height; ni, oi = ni+1, oi+1 {
				if newToOld[ni] >= 0 {
					break // Already matched
				}
				if new.hashes[ni] == old.hashes[oi] {
					newToOld[ni] = oi
				} else {
					break
				}
			}
			// Expand backward
			for ni, oi := i-1, newToOld[i]-1; ni >= 0 && oi >= 0; ni, oi = ni-1, oi-1 {
				if newToOld[ni] >= 0 {
					break // Already matched
				}
				if new.hashes[ni] == old.hashes[oi] {
					newToOld[ni] = oi
				} else {
					break
				}
			}
		}
	}

	// Find the largest contiguous block with consistent offset
	// A "hunk" is a range of new lines that all map to old lines with the same shift
	bestStart := -1
	bestSize := 0
	bestShift := 0

	i := 0
	for i < height {
		if newToOld[i] < 0 {
			i++
			continue
		}

		// Start of a potential hunk
		start := i
		shift := newToOld[i] - i
		size := 1

		// Extend while consecutive and same shift
		for i+size < height && newToOld[i+size] == newToOld[i]+size {
			size++
		}

		// ncurses validation: require 3+ lines, and shift not too large relative to size
		// Formula: size >= 3 && size + min(size/8, 2) >= abs(shift)
		minExtra := max(size/8, 2)
		absShift := shift
		if absShift < 0 {
			absShift = -absShift
		}

		if size >= 3 && size+minExtra >= absShift {
			if size > bestSize {
				bestStart = start
				bestSize = size
				bestShift = shift
			}
		}

		i += size
	}

	if bestStart < 0 || bestShift == 0 {
		return nil
	}

	// Convert to scroll operation
	// bestShift > 0 means old lines were higher (scroll up to see them)
	// bestShift < 0 means old lines were lower (scroll down to see them)
	if bestShift > 0 {
		// Content moved up - scroll up
		return &ScrollOp{
			Direction: 1,
			Amount:    bestShift,
			Top:       bestStart,
			Bottom:    bestStart + bestSize - 1 + bestShift,
		}
	} else {
		// Content moved down - scroll down
		return &ScrollOp{
			Direction: -1,
			Amount:    -bestShift,
			Top:       bestStart + bestShift,
			Bottom:    bestStart + bestSize - 1,
		}
	}
}

// LineChange represents a change to a single line.
type LineChange struct {
	Row      int
	Spans    []ChangeSpan
	ClearEOL bool // Whether to clear to end of line after last span
	ClearCol int  // Column to position cursor before clearing (used when spans is empty)
}

// ChangeSpan represents a contiguous span of changed cells.
type ChangeSpan struct {
	Col   int
	Cells []Cell
}

// diffLines computes the changes needed to transform old line to new line.
// Returns nil if the lines are identical.
func diffLine(row int, old, new []Cell) *LineChange {
	if len(old) != len(new) {
		// Different widths - rewrite whole line
		return &LineChange{
			Row: row,
			Spans: []ChangeSpan{
				{Col: 0, Cells: new},
			},
		}
	}

	// Find changed regions
	var spans []ChangeSpan
	inChange := false
	changeStart := 0

	for i := range len(old) {
		if !old[i].Equal(new[i]) {
			if !inChange {
				inChange = true
				changeStart = i
			}
		} else {
			if inChange {
				// End of changed region
				spans = append(spans, ChangeSpan{
					Col:   changeStart,
					Cells: new[changeStart:i],
				})
				inChange = false
			}
		}
	}
	// Handle final change region
	if inChange {
		spans = append(spans, ChangeSpan{
			Col:   changeStart,
			Cells: new[changeStart:],
		})
	}

	if len(spans) == 0 {
		return nil // No changes
	}

	// Optimization: check if it's cheaper to clear the line and rewrite
	// vs. doing sparse updates
	const cursorMoveCost = 6 // approximate bytes for a cursor position sequence
	sparseLen := 0
	for _, span := range spans {
		sparseLen += cursorMoveCost + len(span.Cells)
	}
	fullLen := len(new)

	// Determine if we should clear to EOL
	// If the last span extends to the end and new line has trailing spaces
	// we can use clear-to-EOL instead of writing spaces
	clearEOL := false
	clearCol := 0
	if len(spans) > 0 {
		lastSpan := spans[len(spans)-1]
		lastEnd := lastSpan.Col + len(lastSpan.Cells)
		if lastEnd == len(new) {
			// Check if trailing cells are default-styled spaces
			for i := len(lastSpan.Cells) - 1; i >= 0; i-- {
				c := lastSpan.Cells[i]
				if c.Rune == ' ' && c.Style.IsDefault() {
					clearEOL = true
					lastSpan.Cells = lastSpan.Cells[:i]
				} else {
					break
				}
			}
			if len(lastSpan.Cells) == 0 {
				// Track where to clear from before removing the span
				clearCol = lastSpan.Col
				spans = spans[:len(spans)-1]
			} else {
				spans[len(spans)-1] = lastSpan
			}
		}
	}

	// If sparse updates cost more than full line rewrite (with some margin),
	// just rewrite the whole line
	if sparseLen > fullLen*2 && len(spans) > 2 {
		return &LineChange{
			Row: row,
			Spans: []ChangeSpan{
				{Col: 0, Cells: new},
			},
		}
	}

	return &LineChange{
		Row:      row,
		Spans:    spans,
		ClearEOL: clearEOL,
		ClearCol: clearCol,
	}
}

// Diff computes all the changes needed to transform old buffer to new buffer.
// It returns the escape sequences and content to write to the terminal.
func Diff(old, new *Buffer, currentStyle *Style) (string, Style) {
	var out strings.Builder
	style := *currentStyle

	// Scroll detection is expensive. Only do it when there are many changed lines,
	// which suggests content may have scrolled.
	changedLines := 0
	for i := 0; i < new.Height && i < old.Height; i++ {
		if old.hashes[i] != new.hashes[i] {
			changedLines++
		}
	}

	// Only attempt scroll detection if many lines changed (potential scroll)
	if changedLines > new.Height/2 {
		scrollOp := detectScroll(old, new)
		if scrollOp != nil {
			// Apply scroll
			scrollSeq := applyScroll(scrollOp, new.Height)
			out.WriteString(scrollSeq)

			// After scroll, update our understanding of what's on screen
			// by simulating the scroll on the old buffer
			old = simulateScroll(old, scrollOp)
		}
	}

	// Now diff line by line
	for row := range new.Height {
		if row >= old.Height {
			// New line - write it all
			change := &LineChange{
				Row: row,
				Spans: []ChangeSpan{
					{Col: 0, Cells: new.Cells[row]},
				},
			}
			seq, newStyle := renderChange(change, style, new.Width)
			out.WriteString(seq)
			style = newStyle
			continue
		}

		// Check if lines are identical via hash
		// djb2 hash - collisions are astronomically unlikely
		if old.hashes[row] == new.hashes[row] {
			continue // No change
		}

		// Compute line diff
		change := diffLine(row, old.Cells[row], new.Cells[row])
		if change != nil {
			seq, newStyle := renderChange(change, style, new.Width)
			out.WriteString(seq)
			style = newStyle
		}
	}

	return out.String(), style
}

// applyScroll generates the escape sequences for a scroll operation.
func applyScroll(op *ScrollOp, screenHeight int) string {
	var out strings.Builder

	if op.Top != 0 || op.Bottom != screenHeight-1 {
		writeScrollRegion(&out, op.Top+1, op.Bottom+1)
	}

	writeCursorPosition(&out, op.Top+1, 1)

	writeScrollAmount(&out, op.Direction, op.Amount)

	if op.Top != 0 || op.Bottom != screenHeight-1 {
		out.WriteString(ScrollRegionReset)
	}

	return out.String()
}

// simulateScroll applies a scroll operation to a buffer, returning a new buffer
// representing what would be on screen after scrolling. Row slices are shared
// with the original for moved rows; only vacated rows are newly allocated.
// This is safe because the result is only used for reading during diff.
func simulateScroll(buf *Buffer, op *ScrollOp) *Buffer {
	result := &Buffer{
		Width:  buf.Width,
		Height: buf.Height,
		Cells:  make([][]Cell, buf.Height),
		hashes: make([]uint64, buf.Height),
	}

	copy(result.Cells, buf.Cells)
	copy(result.hashes, buf.hashes)

	if op.Direction > 0 {
		for i := op.Top; i <= op.Bottom-op.Amount; i++ {
			result.Cells[i] = buf.Cells[i+op.Amount]
			result.hashes[i] = buf.hashes[i+op.Amount]
		}
		for i := op.Bottom - op.Amount + 1; i <= op.Bottom; i++ {
			result.Cells[i] = makeEmptyRow(buf.Width)
			result.hashes[i] = hashLine(result.Cells[i])
		}
	} else {
		for i := op.Bottom; i >= op.Top+op.Amount; i-- {
			result.Cells[i] = buf.Cells[i-op.Amount]
			result.hashes[i] = buf.hashes[i-op.Amount]
		}
		for i := op.Top; i < op.Top+op.Amount; i++ {
			result.Cells[i] = makeEmptyRow(buf.Width)
			result.hashes[i] = hashLine(result.Cells[i])
		}
	}

	return result
}

func makeEmptyRow(width int) []Cell {
	row := make([]Cell, width)
	empty := EmptyCell()
	for j := range row {
		row[j] = empty
	}
	return row
}

// renderChange generates the escape sequences to apply a line change.
func renderChange(change *LineChange, currentStyle Style, width int) (string, Style) {
	var out strings.Builder
	style := currentStyle

	for _, span := range change.Spans {
		writeCursorPosition(&out, change.Row+1, span.Col+1)
		style = writeCells(&out, span.Cells, style)
	}

	if change.ClearEOL {
		if len(change.Spans) == 0 {
			writeCursorPosition(&out, change.Row+1, change.ClearCol+1)
		}
		if !style.IsDefault() {
			out.WriteString(SGRReset)
			style = DefaultStyle()
		}
		out.WriteString(EraseLineRight)
	}

	return out.String(), style
}

// FullRedraw generates the escape sequences to completely redraw the screen.
func FullRedraw(buf *Buffer) string {
	var out strings.Builder
	style := DefaultStyle()

	out.WriteString(SGRReset)
	out.WriteString(CursorHome)
	out.WriteString(EraseScreen)

	for row := range buf.Height {
		writeCursorPosition(&out, row+1, 1)
		style = writeCells(&out, buf.Cells[row], style)
	}

	if !style.IsDefault() {
		out.WriteString(SGRReset)
	}

	return out.String()
}

// Helpers

// writeCells writes cells to the builder, handling style transitions and special cell types.
func writeCells(out *strings.Builder, cells []Cell, style Style) Style {
	for _, cell := range cells {
		if cell.Style != style {
			out.WriteString(sgrSequence(style, cell.Style))
			style = cell.Style
		}
		if cell.Width == 0 {
			continue
		}
		if cell.Rune == 0 {
			out.WriteRune(' ')
		} else {
			out.WriteRune(cell.Rune)
		}
	}
	return style
}

// writeCursorPosition writes a cursor position escape sequence.
func writeCursorPosition(out *strings.Builder, row, col int) {
	out.WriteString(CSI)
	out.WriteString(strconv.Itoa(row))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(col))
	out.WriteByte('H')
}

// writeScrollRegion writes a set-scroll-region escape sequence.
func writeScrollRegion(out *strings.Builder, top, bottom int) {
	out.WriteString(CSI)
	out.WriteString(strconv.Itoa(top))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(bottom))
	out.WriteByte('r')
}

// writeScrollAmount writes a scroll up or down escape sequence.
func writeScrollAmount(out *strings.Builder, direction, amount int) {
	out.WriteString(CSI)
	out.WriteString(strconv.Itoa(amount))
	if direction > 0 {
		out.WriteByte('S')
	} else {
		out.WriteByte('T')
	}
}
