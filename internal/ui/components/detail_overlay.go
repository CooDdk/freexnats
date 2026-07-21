package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/CooDdk/freexnats/internal/ui"
)

type DetailRow struct {
	Label string
	Value string
}

type DetailOverlay struct {
	visible bool
	title   string
	rows    []DetailRow
}

func NewDetailOverlay() *DetailOverlay {
	return &DetailOverlay{}
}

func (o *DetailOverlay) Show(title string, rows []DetailRow) {
	o.title = title
	o.rows = rows
	o.visible = true
}

func (o *DetailOverlay) Hide() {
	o.visible = false
}

func (o *DetailOverlay) Visible() bool {
	return o.visible
}

// PlaceOn composes the overlay on top of the given background string.
// The background is always padded to `height` lines so toggling the overlay
// does not shift downstream layout.
func (o *DetailOverlay) PlaceOn(bg string, width, height int) string {
	bg = padToHeight(bg, height)
	if !o.visible {
		return bg
	}
	box := o.renderBox(width)
	return placeOverlay(box, bg, width, height)
}

func padToHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) >= height {
		return s
	}
	pad := make([]string, height-len(lines))
	return s + "\n" + strings.Join(pad, "\n")
}

func (o *DetailOverlay) renderBox(maxWidth int) string {
	boxWidth := maxWidth - 6
	if boxWidth > 80 {
		boxWidth = 80
	}
	if boxWidth < 30 {
		boxWidth = 30
	}
	labelW := 16
	valueW := boxWidth - labelW - 4
	if valueW < 12 {
		valueW = 12
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(ui.BrandPrimary).
		Bold(true)
	labelStyle := lipgloss.NewStyle().
		Foreground(ui.SubtleColor).
		Width(labelW)
	valueStyle := lipgloss.NewStyle().
		Foreground(ui.TextColor)

	var lines []string
	lines = append(lines, titleStyle.Render(o.title))
	lines = append(lines, lipgloss.NewStyle().Foreground(ui.BorderColor).Render(strings.Repeat("─", boxWidth-2)))

	for _, row := range o.rows {
		wrapped := wrapValue(row.Value, valueW)
		parts := strings.Split(wrapped, "\n")
		for i, p := range parts {
			label := ""
			if i == 0 {
				label = labelStyle.Render(row.Label)
			} else {
				label = labelStyle.Render("")
			}
			lines = append(lines, label+valueStyle.Render(p))
		}
	}

	hint := lipgloss.NewStyle().Foreground(ui.TextFaint).Render("Esc / click again: close")
	lines = append(lines, "")
	lines = append(lines, hint)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.BrandPrimary).
		Background(ui.BgLightColor).
		Padding(1, 2).
		Width(boxWidth).
		Render(strings.Join(lines, "\n"))
}

// placeOverlay splices `overlay` onto `bg` at a centered position.
// Both are already-rendered ANSI strings. Preserves background around the overlay.
func placeOverlay(overlay, bg string, width, height int) string {
	overlayLines := strings.Split(overlay, "\n")
	bgLines := strings.Split(bg, "\n")

	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}

	overlayW := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > overlayW {
			overlayW = w
		}
	}
	overlayH := len(overlayLines)

	x := (width - overlayW) / 2
	if x < 0 {
		x = 0
	}
	y := (height - overlayH) / 2
	if y < 0 {
		y = 0
	}

	for i, ovLine := range overlayLines {
		bgIdx := y + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}
		bgLines[bgIdx] = spliceLine(x, ovLine, bgLines[bgIdx])
	}
	return strings.Join(bgLines, "\n")
}

// PlaceOverlayAt splices `overlay` onto `bg` at the caller-supplied (x, y)
// origin. Unlike placeOverlay it does NOT center — callers use this for
// anchored popups (dropdowns, tooltips) that sit next to a specific widget.
// The background is padded with empty lines to fit if it's shorter than
// y + overlay height so the splice never truncates.
func PlaceOverlayAt(bg, overlay string, x, y int) string {
	overlayLines := strings.Split(overlay, "\n")
	bgLines := strings.Split(bg, "\n")

	needed := y + len(overlayLines)
	for len(bgLines) < needed {
		bgLines = append(bgLines, "")
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	for i, ovLine := range overlayLines {
		bgIdx := y + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}
		bgLines[bgIdx] = spliceLine(x, ovLine, bgLines[bgIdx])
	}
	return strings.Join(bgLines, "\n")
}

// spliceLine overlays fg on top of bg starting at column x.
func spliceLine(x int, fg, bg string) string {
	fgW := ansi.StringWidth(fg)

	// Ensure bg is at least x cells wide by padding with spaces
	bgW := ansi.StringWidth(bg)
	if bgW < x+fgW {
		bg = bg + strings.Repeat(" ", x+fgW-bgW)
	}

	left := ansi.Truncate(bg, x, "")
	leftW := ansi.StringWidth(left)
	if leftW < x {
		left += strings.Repeat(" ", x-leftW)
	}

	right := ansi.TruncateLeft(bg, x+fgW, "")

	return left + "\x1b[0m" + fg + "\x1b[0m" + right
}

func wrapValue(s string, width int) string {
	if width <= 0 || runewidth.StringWidth(s) <= width {
		return s
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, wrapValueLine(line, width))
	}
	return strings.Join(out, "\n")
}

func wrapValueLine(s string, width int) string {
	if width <= 0 || runewidth.StringWidth(s) <= width {
		return s
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if cur+rw > width {
			b.WriteByte('\n')
			cur = 0
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String()
}
