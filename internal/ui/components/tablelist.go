package components

import (
	"strconv"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/CooDdk/freexnats/internal/ui"
)

type Column struct {
	Title    string
	MinWidth int
	Flex     int
}

type MouseAction int

const (
	MouseNone MouseAction = iota
	MouseScrolled
	MouseRowClicked
)

var tableCounter int64

type TableList struct {
	columns      []Column
	rows         [][]string
	selectedIdx  int
	offset       int
	width        int
	height       int
	searchQuery  string
	filteredRows []int
	zonePrefix   string
	// focused mirrors the keyboard-focus state managed by focus.Manager.
	// Row highlight already indicates the selected row; when focused=false
	// the selection dims to a muted variant so the user can tell which
	// chrome region has keyboard input.
	focused bool

	// actionHints is a list of "key: label" strings surfaced inline under
	// the selected row (using the spacer slot that already exists between
	// rows). Empty = no hint bar. Rendered muted so it doesn't fight the
	// selection color for attention; only shown when the table is focused
	// so the user knows the keys will actually fire.
	actionHints []string
}

func NewTableList(columns []Column) *TableList {
	id := atomic.AddInt64(&tableCounter, 1)
	return &TableList{
		columns:    columns,
		zonePrefix: "table-" + strconv.FormatInt(id, 10) + "-row-",
	}
}

func (t *TableList) rowZoneID(rowIdx int) string {
	return t.zonePrefix + strconv.Itoa(rowIdx)
}

func (t *TableList) SetRows(rows [][]string) {
	t.rows = rows
	t.selectedIdx = 0
	t.offset = 0
	t.updateFilteredRows()
}

func (t *TableList) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func (t *TableList) SelectedIndex() int {
	if len(t.filteredRows) == 0 {
		return -1
	}
	if t.selectedIdx >= len(t.filteredRows) {
		return t.filteredRows[len(t.filteredRows)-1]
	}
	return t.filteredRows[t.selectedIdx]
}

func (t *TableList) SelectedRow() []string {
	idx := t.SelectedIndex()
	if idx < 0 || idx >= len(t.rows) {
		return nil
	}
	return t.rows[idx]
}

func (t *TableList) MoveUp() {
	if t.selectedIdx > 0 {
		t.selectedIdx--
	}
	t.adjustOffset()
}

func (t *TableList) MoveDown() {
	if t.selectedIdx < len(t.filteredRows)-1 {
		t.selectedIdx++
	}
	t.adjustOffset()
}

func (t *TableList) MovePageUp() {
	pageSize := t.visibleRows()
	t.selectedIdx -= pageSize
	if t.selectedIdx < 0 {
		t.selectedIdx = 0
	}
	t.adjustOffset()
}

func (t *TableList) MovePageDown() {
	pageSize := t.visibleRows()
	t.selectedIdx += pageSize
	if t.selectedIdx >= len(t.filteredRows) {
		t.selectedIdx = len(t.filteredRows) - 1
		if t.selectedIdx < 0 {
			t.selectedIdx = 0
		}
	}
	t.adjustOffset()
}

func (t *TableList) GoTop() {
	t.selectedIdx = 0
	t.offset = 0
}

func (t *TableList) GoBottom() {
	t.selectedIdx = len(t.filteredRows) - 1
	if t.selectedIdx < 0 {
		t.selectedIdx = 0
	}
	t.adjustOffset()
}

func (t *TableList) SetSearchQuery(query string) {
	t.searchQuery = query
	t.selectedIdx = 0
	t.offset = 0
	t.updateFilteredRows()
}

func (t *TableList) updateFilteredRows() {
	t.filteredRows = nil
	if t.searchQuery == "" {
		for i := range t.rows {
			t.filteredRows = append(t.filteredRows, i)
		}
		return
	}
	query := strings.ToLower(t.searchQuery)
	for i, row := range t.rows {
		rowText := strings.ToLower(strings.Join(row, " "))
		if strings.Contains(rowText, query) {
			t.filteredRows = append(t.filteredRows, i)
		}
	}
	if t.selectedIdx >= len(t.filteredRows) {
		t.selectedIdx = 0
	}
}

func (t *TableList) visibleRows() int {
	rows := (t.height - 3) / 2
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (t *TableList) adjustOffset() {
	visible := t.visibleRows()
	if visible <= 0 {
		return
	}

	if t.selectedIdx < t.offset {
		t.offset = t.selectedIdx
	}
	if t.selectedIdx >= t.offset+visible {
		t.offset = t.selectedIdx - visible + 1
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *TableList) View() string {
	if len(t.rows) == 0 {
		return lipgloss.NewStyle().
			Foreground(ui.TextFaint).
			Italic(true).
			Render("  No items to display")
	}

	header := t.renderHeader()
	body := t.renderBody()

	return header + "\n" + body
}

func (t *TableList) columnWidths() []int {
	n := len(t.columns)
	widths := make([]int, n)
	if n == 0 || t.width <= 0 {
		return widths
	}
	totalMin := 0
	totalFlex := 0
	for i, col := range t.columns {
		min := col.MinWidth
		if min <= 0 {
			min = 12
		}
		widths[i] = min
		totalMin += min
		totalFlex += col.Flex
	}
	extra := t.width - totalMin
	if extra <= 0 {
		return widths
	}
	if totalFlex == 0 {
		widths[n-1] += extra
		return widths
	}
	consumed := 0
	lastFlexIdx := -1
	for i, col := range t.columns {
		if col.Flex > 0 {
			share := extra * col.Flex / totalFlex
			widths[i] += share
			consumed += share
			lastFlexIdx = i
		}
	}
	if consumed < extra && lastFlexIdx >= 0 {
		widths[lastFlexIdx] += extra - consumed
	}
	return widths
}

func (t *TableList) renderHeader() string {
	widths := t.columnWidths()
	var parts []string
	for i, col := range t.columns {
		w := widths[i]

		titleStyle := lipgloss.NewStyle().
			Foreground(ui.PrimaryLight).
			Background(ui.BgLightColor).
			Bold(true).
			Width(w).
			Padding(0, 2)

		parts = append(parts, titleStyle.Render(truncCell(col.Title, w-4)))
	}

	return strings.Join(parts, "")
}

func (t *TableList) renderBody() string {
	visible := t.visibleRows()
	if visible <= 0 {
		return ""
	}

	var lines []string
	end := t.offset + visible
	if end > len(t.filteredRows) {
		end = len(t.filteredRows)
	}

	cellStyle := func(isSelected bool, rowIdx int) lipgloss.Style {
		if isSelected {
			// When the table has keyboard focus, use the full Primary
			// selection color so the current row lights up clearly. When
			// unfocused (another chrome region owns focus), keep the row
			// highlighted but muted so the user can still tell which row
			// is current without confusing it for "here's where typing
			// goes."
			if t.focused {
				return lipgloss.NewStyle().
					Foreground(ui.SelectionFg).
					Background(ui.Primary).
					Bold(true)
			}
			return lipgloss.NewStyle().
				Foreground(ui.TextColor).
				Background(ui.SelectionBg).
				Bold(true)
		}
		bg := ui.BgColor
		if rowIdx%2 == 1 {
			bg = ui.BgLightColor
		}
		return lipgloss.NewStyle().
			Foreground(ui.TextColor).
			Background(bg)
	}

	widths := t.columnWidths()
	for i := t.offset; i < end; i++ {
		rowIdx := t.filteredRows[i]
		row := t.rows[rowIdx]
		isSelected := i == t.selectedIdx

		var parts []string
		for j := range t.columns {
			w := widths[j]

			cellText := ""
			if j < len(row) {
				cellText = row[j]
			}

			s := cellStyle(isSelected, i)
			parts = append(parts,
				s.Width(w).Padding(0, 2).Render(truncCell(cellText, w-4)),
			)
		}

		lines = append(lines, zone.Mark(t.rowZoneID(rowIdx), strings.Join(parts, "")))

		if i < end-1 || len(lines) < visible*2 {
			bg := ui.BgColor
			if i%2 == 1 {
				bg = ui.BgLightColor
			}
			// For the selected row, replace the trailing spacer with an
			// inline action-hint bar — right-aligned, muted, and only when
			// the table has keyboard focus. Row bg stays consistent so the
			// hint reads as attached to the row above.
			if isSelected && t.focused && len(t.actionHints) > 0 {
				lines = append(lines, t.renderHintBar(bg))
			} else {
				spacer := lipgloss.NewStyle().
					Background(bg).
					Width(t.width).
					Render("")
				lines = append(lines, spacer)
			}
		}
	}

	for len(lines) < visible*2 {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (t *TableList) TotalItems() int {
	return len(t.filteredRows)
}

// renderHintBar builds the inline hint bar for the selected row. It sits in
// the spacer slot immediately below the row, right-aligned within the same
// bg so it visually belongs to the row. Each hint is styled as "key" bold
// primary-light + "label" muted, joined with a middle dot.
func (t *TableList) renderHintBar(bg lipgloss.TerminalColor) string {
	keyStyle := lipgloss.NewStyle().Foreground(ui.PrimaryLight).Bold(true).Background(bg)
	labelStyle := lipgloss.NewStyle().Foreground(ui.TextMuted).Background(bg)
	sepStyle := lipgloss.NewStyle().Foreground(ui.TextFaint).Background(bg)

	var chips []string
	for _, h := range t.actionHints {
		key, label, ok := strings.Cut(h, ":")
		if !ok {
			chips = append(chips, labelStyle.Render(strings.TrimSpace(h)))
			continue
		}
		chips = append(chips, keyStyle.Render(strings.TrimSpace(key))+labelStyle.Render(" "+strings.TrimSpace(label)))
	}
	sep := sepStyle.Render("  ·  ")
	inner := strings.Join(chips, sep)
	// Pad on the left so the chip cluster right-aligns; reserve 2 cols on
	// the right for visual breathing room.
	pad := t.width - ansi.StringWidth(inner) - 2
	if pad < 1 {
		pad = 1
	}
	padStr := lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
	tail := lipgloss.NewStyle().Background(bg).Render("  ")
	return padStr + inner + tail
}

// AtTop reports whether the row cursor is on the first row. Used by the
// focus manager to decide when ↑ escapes to the previous chrome element.
func (t *TableList) AtTop() bool {
	return t.selectedIdx <= 0 || len(t.filteredRows) == 0
}

// AtBottom reports whether the row cursor is on the last row. Used by the
// focus manager to decide when ↓ escapes to the next chrome element.
func (t *TableList) AtBottom() bool {
	return len(t.filteredRows) == 0 || t.selectedIdx >= len(t.filteredRows)-1
}

// Focus / Blur toggle the row-highlight brightness. Focused = full Primary
// row; blurred = muted selection tint (still visible so the user can see
// their cursor position while another chrome region has input).
func (t *TableList) Focus() { t.focused = true }
func (t *TableList) Blur()  { t.focused = false }

// SetActionHints installs the per-row hint bar that appears under the
// selected row (in the spacer between rows). Each entry should be "key:
// label" (e.g. "Enter: View", "d: Delete"). Pass nil / empty to hide.
func (t *TableList) SetActionHints(hints []string) { t.actionHints = hints }

func (t *TableList) PositionText() string {
	if len(t.filteredRows) == 0 {
		return "0/0"
	}
	return formatInt(t.selectedIdx+1) + "/" + formatInt(len(t.filteredRows))
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// truncCell truncates cell text to `w` display columns while preserving any
// embedded ANSI SGR sequences. Callers can pass a plain string or a lipgloss-
// rendered string (e.g. a colored severity marker); the display width is
// measured on printable cells only.
func truncCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		// Pad to full width in plain space so alignment stays stable when the
		// cell contains colored inner spans (which don't add width).
		return s + strings.Repeat(" ", w-ansi.StringWidth(s))
	}
	if w == 1 {
		return "…"
	}
	// ANSI-aware truncate. runewidth kept for safety on very old strings that
	// contain no escapes — the ansi path is a superset of the plain path.
	trunc := ansi.Truncate(s, w-1, "")
	if ansi.StringWidth(trunc) < w {
		trunc += "…"
	}
	// Final pad to exact width.
	if pad := w - ansi.StringWidth(trunc); pad > 0 {
		trunc += strings.Repeat(" ", pad)
	}
	return trunc
}

func (t *TableList) HandleMouse(msg tea.MouseMsg) MouseAction {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		t.MoveUp()
		return MouseScrolled
	case tea.MouseButtonWheelDown:
		t.MoveDown()
		return MouseScrolled
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return MouseNone
	}
	visible := t.visibleRows()
	end := t.offset + visible
	if end > len(t.filteredRows) {
		end = len(t.filteredRows)
	}
	// Try reported Y first, then ±1 as tolerance for terminals where
	// mouse.Y comes back one row off from the visual row.
	for _, dy := range []int{0, 1, -1} {
		probe := msg
		probe.Y = msg.Y + dy
		for i := t.offset; i < end; i++ {
			rowIdx := t.filteredRows[i]
			if zone.Get(t.rowZoneID(rowIdx)).InBounds(probe) {
				t.selectedIdx = i
				t.adjustOffset()
				return MouseRowClicked
			}
		}
	}
	return MouseNone
}
