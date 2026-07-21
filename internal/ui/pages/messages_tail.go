package pages

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/pkg/utils"
)

const (
	tailBufferCap = 500
	tailPollEvery = 200 * time.Millisecond
)

// TailState owns the live-tail subscription and its rendering. Held by
// MessagesPage; created on entering ModeTail, torn down on leaving.
type TailState struct {
	stream  *natsclient.StreamInfo
	sub     *natsclient.TailSub
	err     string
	paused  bool
	autoBot bool // auto-scroll to newest when not paused

	// Rolling rate: last 5 poll deltas so we average over ~1 second.
	lastPollTotal uint64
	lastPollTime  time.Time
	rateSamples   []float64

	scroll int // top row of visible window (from oldest end)

	// Layout: absolute origin of the tail panel's top-left. Set by SetOrigin
	// every render so hit-testing on Pause/Clear/Back matches the visible
	// position.
	topX, topY int
	width      int
	height     int

	// Hit boxes recorded during View() for mouse handling.
	btnRowY            int
	pauseX, pauseEndX  int
	clearX, clearEndX  int
	backX, backEndX    int

	// Filter: case-insensitive substring match on Subject OR Data. Committed
	// via Enter, cancelled via Esc; while focused the textinput swallows keys.
	filter        string
	filterInput   textinput.Model
	filterFocused bool
	filterRowY    int
	filterX       int
	filterEndX    int
}

// TailPollMsg wakes the tail up to drain the ring buffer.
type TailPollMsg struct{}

// TailBackToHistoryMsg is emitted when the user clicks the History segment.
type TailBackToHistoryMsg struct{}

func newTailState(stream *natsclient.StreamInfo) *TailState {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "filter subject or payload (substring)"
	ti.CharLimit = 128
	ti.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(ui.SelectionFg).Background(ui.Primary)
	ti.Cursor.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)

	return &TailState{
		stream:      stream,
		autoBot:     true,
		filterInput: ti,
	}
}

// Start subscribes to the stream's declared subjects. Returns a Cmd that
// schedules the first poll.
func (t *TailState) Start(client *natsclient.Client) tea.Cmd {
	if t.stream == nil {
		return nil
	}
	subjects := t.stream.Subjects
	if len(subjects) == 0 {
		t.err = "stream has no declared subjects"
		return nil
	}
	sub, err := client.NewTailSub(subjects, tailBufferCap)
	if err != nil {
		t.err = err.Error()
		return nil
	}
	t.sub = sub
	t.lastPollTime = time.Now()
	return tailPollCmd()
}

// Stop unsubscribes and releases the subscription. Safe to call multiple times.
func (t *TailState) Stop() {
	if t.sub != nil {
		t.sub.Stop()
		t.sub = nil
	}
}

func tailPollCmd() tea.Cmd {
	return tea.Tick(tailPollEvery, func(time.Time) tea.Msg { return TailPollMsg{} })
}

// OnPoll refreshes the rolling rate and returns a Cmd to reschedule.
func (t *TailState) OnPoll() tea.Cmd {
	if t.sub == nil {
		return nil
	}
	now := time.Now()
	total := t.sub.Total()
	dt := now.Sub(t.lastPollTime).Seconds()
	if dt > 0 {
		instant := float64(total-t.lastPollTotal) / dt
		t.rateSamples = append(t.rateSamples, instant)
		if len(t.rateSamples) > 5 {
			t.rateSamples = t.rateSamples[len(t.rateSamples)-5:]
		}
	}
	t.lastPollTotal = total
	t.lastPollTime = now
	return tailPollCmd()
}

func (t *TailState) rate() float64 {
	if len(t.rateSamples) == 0 {
		return 0
	}
	var sum float64
	for _, v := range t.rateSamples {
		sum += v
	}
	return sum / float64(len(t.rateSamples))
}

func (t *TailState) SetSize(w, h int) {
	t.width = w
	t.height = h
}

func (t *TailState) SetOrigin(x, y int) {
	t.topX = x
	t.topY = y
}

// TogglePause flips paused state and freezes auto-scroll.
func (t *TailState) TogglePause() {
	t.paused = !t.paused
	if !t.paused {
		t.autoBot = true
	}
}

// Clear empties the buffer.
func (t *TailState) Clear() {
	if t.sub != nil {
		t.sub.Clear()
	}
	t.scroll = 0
}

// ScrollUp moves the visible window up one line and disables auto-scroll.
func (t *TailState) ScrollUp(n int) {
	if t.scroll < 1_000_000 {
		t.scroll += n
	}
	t.autoBot = false
}

// ScrollDown moves the window down; hitting bottom re-enables auto-scroll.
func (t *TailState) ScrollDown(n int) {
	t.scroll -= n
	if t.scroll <= 0 {
		t.scroll = 0
		t.autoBot = true
	}
}

// StartFilterEdit focuses the filter textinput and pre-fills with the current
// committed filter so the user can extend or clear it.
func (t *TailState) StartFilterEdit() {
	t.filterInput.SetValue(t.filter)
	t.filterInput.CursorEnd()
	t.filterInput.Focus()
	t.filterFocused = true
}

// CommitFilterEdit accepts the textinput value as the live filter.
func (t *TailState) CommitFilterEdit() {
	t.filter = strings.TrimSpace(t.filterInput.Value())
	t.filterInput.Blur()
	t.filterFocused = false
}

// CancelFilterEdit discards the pending edit and restores the previous filter.
func (t *TailState) CancelFilterEdit() {
	t.filterInput.SetValue(t.filter)
	t.filterInput.Blur()
	t.filterFocused = false
}

// ClearFilter drops the active filter and any pending edit.
func (t *TailState) ClearFilter() {
	t.filter = ""
	t.filterInput.SetValue("")
	t.filterInput.Blur()
	t.filterFocused = false
}

// IsFilterFocused reports whether the filter textinput has keyboard focus.
func (t *TailState) IsFilterFocused() bool { return t.filterFocused }

// UpdateFilterInput forwards a message to the underlying textinput. Callers
// should route Enter/Esc themselves — this handles character input, cursor
// movement, and backspace only.
func (t *TailState) UpdateFilterInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	t.filterInput, cmd = t.filterInput.Update(msg)
	return cmd
}

// matches applies the committed filter to a single TailMsg. Empty filter =
// always true. Match is case-insensitive against Subject and Data.
func (t *TailState) matches(m natsclient.TailMsg) bool {
	if t.filter == "" {
		return true
	}
	needle := strings.ToLower(t.filter)
	if strings.Contains(strings.ToLower(m.Subject), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(string(m.Data)), needle) {
		return true
	}
	return false
}

// HandleMouse routes clicks on the button row. Returns (cmd, handled, backRequested).
//   - backRequested=true means "leave Tail mode entirely" (Back button click)
//     — caller resets mode.
func (t *TailState) HandleMouse(msg tea.MouseMsg) (tea.Cmd, bool, bool) {
	// Wheel scroll anywhere in the tail viewport.
	if msg.Button == tea.MouseButtonWheelUp {
		t.ScrollUp(1)
		return nil, true, false
	}
	if msg.Button == tea.MouseButtonWheelDown {
		t.ScrollDown(1)
		return nil, true, false
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false, false
	}
	if inRowTail(msg.Y, t.btnRowY) {
		if msg.X >= t.pauseX && msg.X <= t.pauseEndX {
			t.TogglePause()
			return nil, true, false
		}
		if msg.X >= t.clearX && msg.X <= t.clearEndX {
			t.Clear()
			return nil, true, false
		}
		if msg.X >= t.backX && msg.X <= t.backEndX {
			return nil, true, true
		}
	}
	if inRowTail(msg.Y, t.filterRowY) {
		if msg.X >= t.filterX && msg.X <= t.filterEndX {
			t.StartFilterEdit()
			return nil, true, false
		}
	}
	return nil, false, false
}

func inRowTail(y, target int) bool {
	return y >= target-1 && y <= target+1
}

// View renders the tail panel + scrolling message list. The panel is rendered
// with a rounded border so it visually reads as a distinct workspace.
func (t *TailState) View() string {
	w := t.width - 4
	if w < 30 {
		w = 30
	}

	// Row 1: status pill · rate · count on left; stream name on right.
	statusPill := lipgloss.NewStyle().
		Foreground(ui.SelectionFg).
		Background(ui.Success).
		Bold(true).
		Padding(0, 1).
		Render("● LIVE")
	if t.paused {
		statusPill = lipgloss.NewStyle().
			Foreground(ui.SelectionFg).
			Background(ui.Warning).
			Bold(true).
			Padding(0, 1).
			Render("‖ PAUSED")
	}
	if t.err != "" {
		statusPill = lipgloss.NewStyle().
			Foreground(ui.SelectionFg).
			Background(ui.Error).
			Bold(true).
			Padding(0, 1).
			Render("✖ ERROR")
	}

	rate := lipgloss.NewStyle().Foreground(ui.PrimaryLight).Bold(true).
		Render(fmt.Sprintf("%.0f msg/s", t.rate()))
	bufN := 0
	matchN := 0
	if t.sub != nil {
		msgs := t.sub.Snapshot()
		bufN = len(msgs)
		if t.filter == "" {
			matchN = bufN
		} else {
			for _, m := range msgs {
				if t.matches(m) {
					matchN++
				}
			}
		}
	}
	var count string
	if t.filter == "" {
		count = lipgloss.NewStyle().Foreground(ui.TextMuted).
			Render(fmt.Sprintf("%d/%d", bufN, tailBufferCap))
	} else {
		count = lipgloss.NewStyle().Foreground(ui.TextMuted).
			Render(fmt.Sprintf("%d matched · %d/%d", matchN, bufN, tailBufferCap))
	}
	sep := lipgloss.NewStyle().Foreground(ui.TextFaint).Render("  ·  ")

	leftMetrics := statusPill + "  " + rate + sep + count

	streamName := lipgloss.NewStyle().Foreground(ui.TextFaint).
		Render(fmt.Sprintf("stream: %s", t.stream.Name))

	leftW := lipgloss.Width(leftMetrics)
	rightW := lipgloss.Width(streamName)
	gap := w - leftW - rightW - 2 // account for panel padding (0,1) → 2 total
	if gap < 1 {
		gap = 1
	}
	row1 := leftMetrics + strings.Repeat(" ", gap) + streamName

	// Row 2: Pause / Clear / Back buttons.
	pauseLabel := "Pause"
	if t.paused {
		pauseLabel = "Resume"
	}
	pauseBtn := renderTailBtn(pauseLabel, t.paused)
	clearBtn := renderTailBtn("Clear", false)
	backBtn := renderTailBtn("Back", false)
	row2 := pauseBtn + " " + clearBtn + " " + backBtn

	// Row 3: filter input. Compact label + textinput; when unfocused shows the
	// committed filter (or the placeholder) so users can tell at a glance
	// whether filtering is on.
	filterLabelStyle := lipgloss.NewStyle().Foreground(ui.TextMuted).Bold(true)
	filterLabel := filterLabelStyle.Render("/ filter ")
	// Inline "clear" hint when a filter is active
	activeHint := ""
	if t.filter != "" && !t.filterFocused {
		activeHint = "  " + lipgloss.NewStyle().Foreground(ui.Warning).Italic(true).
			Render("(active — press / to edit, Esc to clear)")
	}
	// Size the textinput to fit the remaining panel width.
	inputW := w - lipgloss.Width(filterLabel) - lipgloss.Width(activeHint) - 2
	if inputW < 12 {
		inputW = 12
	}
	t.filterInput.Width = inputW
	inputView := t.filterInput.View()
	if t.filterFocused {
		inputView = lipgloss.NewStyle().
			Foreground(ui.TextColor).
			Background(ui.BgLighter).
			Render(inputView)
	}
	row3 := filterLabel + inputView + activeHint

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.BorderColor).
		Padding(0, 1).
		Width(w)
	panel := panelStyle.Render(row1 + "\n" + row2 + "\n" + row3)

	// Record hit-test coordinates. Panel top border sits at topY;
	// row1 is at topY+1 (border row + content). Padding(0,1) shifts X by 1
	// (left border + 1 pad col) → contentStartX = topX + 2.
	contentStartX := t.topX + 2
	t.btnRowY = t.topY + 2
	t.pauseX = contentStartX
	t.pauseEndX = t.pauseX + lipgloss.Width(pauseBtn) - 1
	t.clearX = t.pauseEndX + 2
	t.clearEndX = t.clearX + lipgloss.Width(clearBtn) - 1
	t.backX = t.clearEndX + 2
	t.backEndX = t.backX + lipgloss.Width(backBtn) - 1
	t.filterRowY = t.topY + 3
	t.filterX = contentStartX
	t.filterEndX = contentStartX + lipgloss.Width(filterLabel) + inputW - 1

	// Panel is 5 rows tall (2 borders + 3 content). Everything below is
	// the scrolling message list.
	panelRows := 5
	listBudget := t.height - panelRows - 1 // -1 for blank spacer
	if listBudget < 3 {
		listBudget = 3
	}

	list := t.renderList(w, listBudget)

	return panel + "\n\n" + list
}

// renderList produces exactly `budget` lines of message rows, bottom-anchored
// (newest at the bottom) when auto-scrolling; otherwise scrolled by t.scroll.
func (t *TailState) renderList(w, budget int) string {
	if t.err != "" {
		lines := []string{lipgloss.NewStyle().Foreground(ui.Error).Render("  " + t.err)}
		return padOrClip(lines, budget)
	}
	if t.sub == nil {
		return padOrClip([]string{lipgloss.NewStyle().Foreground(ui.TextFaint).
			Italic(true).Render("  starting...")}, budget)
	}
	msgs := t.sub.Snapshot()
	if len(msgs) == 0 {
		return padOrClip([]string{lipgloss.NewStyle().Foreground(ui.TextFaint).
			Italic(true).Render("  Waiting for messages...")}, budget)
	}
	// Apply the committed filter before windowing so scroll math works on the
	// visible (post-filter) list.
	if t.filter != "" {
		filtered := msgs[:0:0]
		for _, m := range msgs {
			if t.matches(m) {
				filtered = append(filtered, m)
			}
		}
		msgs = filtered
		if len(msgs) == 0 {
			hint := fmt.Sprintf("  No messages match %q — press / to edit, Esc to clear", t.filter)
			return padOrClip([]string{lipgloss.NewStyle().Foreground(ui.Warning).
				Italic(true).Render(hint)}, budget)
		}
	}
	// Bottom of the list = newest. Auto-scroll shows the last `budget` messages.
	var start, end int
	if t.autoBot || t.scroll <= 0 {
		end = len(msgs)
		start = end - budget
		if start < 0 {
			start = 0
		}
		t.scroll = 0
	} else {
		end = len(msgs) - t.scroll
		if end < 1 {
			end = 1
			t.scroll = len(msgs) - 1
		}
		start = end - budget
		if start < 0 {
			start = 0
		}
	}

	tsStyle := lipgloss.NewStyle().Foreground(ui.TextFaint)
	subjStyle := lipgloss.NewStyle().Foreground(ui.Accent)
	sizeStyle := lipgloss.NewStyle().Foreground(ui.TextMuted)
	dataStyle := lipgloss.NewStyle().Foreground(ui.TextColor)

	var lines []string
	for i := start; i < end; i++ {
		m := msgs[i]
		ts := m.Received.Format("15:04:05.000")
		subj := m.Subject
		if runewidth.StringWidth(subj) > 24 {
			subj = runewidth.Truncate(subj, 23, "…")
		}
		subj = subj + strings.Repeat(" ", max(0, 24-runewidth.StringWidth(subj)))
		size := fmt.Sprintf("%s", utils.FormatBytes(uint64(len(m.Data))))
		size = size + strings.Repeat(" ", max(0, 7-runewidth.StringWidth(size)))
		// One-line data preview: strip newlines, trim to fit.
		preview := strings.ReplaceAll(string(m.Data), "\n", " ")
		preview = strings.TrimSpace(preview)
		lineBudget := w - 2 - 12 - 1 - 24 - 1 - 7 - 1 // pad,ts,gap,subj,gap,size,gap
		if lineBudget < 4 {
			lineBudget = 4
		}
		if runewidth.StringWidth(preview) > lineBudget {
			preview = runewidth.Truncate(preview, lineBudget-1, "…")
		}
		line := "  " + tsStyle.Render(ts) + " " +
			subjStyle.Render(subj) + " " +
			sizeStyle.Render(size) + " " +
			dataStyle.Render(preview)
		lines = append(lines, line)
	}

	// Scroll indicator when not at the newest end.
	if t.scroll > 0 {
		indicator := lipgloss.NewStyle().Foreground(ui.Warning).Italic(true).
			Render(fmt.Sprintf("  [scrolled +%d — j/PgDn to catch up]", t.scroll))
		lines = append(lines, indicator)
	}

	// Bottom-anchor: pad top rather than bottom so newest stays flush with
	// the border and old rows scroll off the top.
	if len(lines) < budget {
		pad := make([]string, budget-len(lines))
		lines = append(pad, lines...)
	}
	if len(lines) > budget {
		lines = lines[len(lines)-budget:]
	}
	return strings.Join(lines, "\n")
}

// renderSegment renders one half of the History↔Live segmented control.
// Both halves are rendered as pills so the pair reads as a grouped chip
// control; the active side gets ButtonFocused (Primary bg), the inactive
// side stays ButtonIdle (muted). When keyboardFocused is true, the pill's
// body text is underlined via a single lipgloss style pass (not a
// pre-wrapped ANSI label) so the pill body and Powerline caps stay flush.
func renderSegment(label string, active, keyboardFocused bool) string {
	state := components.ButtonIdle
	if active {
		state = components.ButtonFocused
	}
	if keyboardFocused {
		return components.RenderPillUnderlined(label, state)
	}
	return components.RenderPill(label, state)
}

// renderTailBtn is a compact pill for Pause/Clear/Back. Highlighted variant
// (used when Pause has toggled to Resume) uses the warning color to signal
// state change.
func renderTailBtn(label string, highlight bool) string {
	if highlight {
		return components.RenderPill(label, components.ButtonWarning)
	}
	return components.RenderPill(label, components.ButtonIdle)
}
