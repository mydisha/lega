package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// bar draws a horizontal gauge of the given width, filled to frac (0..1).
func bar(t Theme, width int, frac float64, fill lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	frac = min(max(frac, 0), 1)
	n := int(frac*float64(width) + 0.5)
	if n == 0 && frac > 0 {
		n = 1 // anything above zero gets at least one block
	}
	return fill.Render(strings.Repeat("█", n)) + t.Dim.Render(strings.Repeat("░", width-n))
}

// fit cuts s to w cells with an ellipsis and pads it to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
}

// fitMiddle shortens a path by cutting its middle, keeping the start and
// the file name, then pads it to w cells.
func fitMiddle(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw <= w || w < 5 {
		return fit(s, w)
	}
	keep := w - 1
	head := keep / 3
	tail := keep - head
	return ansi.Truncate(s, head, "") + "…" + ansi.TruncateLeft(s, sw-tail, "")
}

// right pads s on the left so it ends at w cells.
func right(s string, w int) string {
	if pad := w - ansi.StringWidth(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// list tracks the cursor and scroll position of a list on screen.
type list struct {
	cursor int
	offset int
}

// move shifts the cursor by delta within n items.
func (l *list) move(delta, n int) {
	l.cursor = min(max(l.cursor+delta, 0), max(n-1, 0))
}

// window returns the visible range [start, end) for a viewport of height
// rows, scrolling only as much as needed to keep the cursor in view.
func (l *list) window(n, height int) (int, int) {
	if height <= 0 {
		return 0, 0
	}
	l.cursor = min(max(l.cursor, 0), max(n-1, 0))
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+height {
		l.offset = l.cursor - height + 1
	}
	l.offset = min(l.offset, max(n-height, 0))
	return l.offset, min(l.offset+height, n)
}
