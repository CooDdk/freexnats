package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/CooDdk/freexnats/internal/ui"
)

// StreamSelector is a two-state widget:
//   - Closed: renders as a pill "Stream: <name> ▾" that the caller places in
//     its top bar. Clicking the pill or Enter (when keyboard-focused) opens
//     the dropdown.
//   - Open: additionally renders a rounded-border dropdown listing all
//     streams. The caller splices the dropdown onto the final view via
//     PlaceOverlayAt(pillX, pillY+1). Keyboard delegates to HandleKey; mouse
//     to HandleMouse. Selecting a row returns the chosen name; Esc / clicking
//     outside cancels.
type StreamSelector struct {
	streams []string
	current string

	open       bool
	kbFocused  bool
	highlight  int // dropdown cursor while open

	// Layout coords recorded during RenderPill for HandleMouse. Pill top-left
	// in the final view.
	pillX, pillY int
	pillWidth    int

	// Dropdown geometry, recorded during RenderDropdown for mouse hit-testing.
	// Absolute coords into the final view.
	ddX, ddY         int
	ddInnerWidth     int
	ddContentY       int // absolute Y of the first row (skipping border)
	ddRowsRendered   int
}

func NewStreamSelector() *StreamSelector {
	return &StreamSelector{}
}

func (s *StreamSelector) SetStreams(names []string) {
	s.streams = append(s.streams[:0], names...)
	if s.highlight >= len(s.streams) {
		s.highlight = 0
	}
	// If current stream disappeared, clear it — caller decides what to do.
	if s.current != "" {
		found := false
		for _, n := range s.streams {
			if n == s.current {
				found = true
				break
			}
		}
		if !found {
			s.current = ""
		}
	}
}

func (s *StreamSelector) SetCurrent(name string) {
	s.current = name
	for i, n := range s.streams {
		if n == name {
			s.highlight = i
			return
		}
	}
}

func (s *StreamSelector) Current() string { return s.current }

func (s *StreamSelector) Open() {
	if len(s.streams) == 0 {
		return
	}
	s.open = true
	// Land cursor on current stream if any.
	for i, n := range s.streams {
		if n == s.current {
			s.highlight = i
			return
		}
	}
	s.highlight = 0
}

func (s *StreamSelector) Close() { s.open = false }

func (s *StreamSelector) IsOpen() bool { return s.open }

func (s *StreamSelector) KeyboardFocused() bool { return s.kbFocused }

func (s *StreamSelector) SetKeyboardFocused(v bool) { s.kbFocused = v }

// RenderPill returns the closed-state pill. Caller must place it and record
// its origin via SetOrigin so mouse hit-testing works.
func (s *StreamSelector) RenderPill() string {
	label := "Stream: " + s.pillLabel() + " ▾"
	state := ButtonIdle
	if s.open {
		state = ButtonFocused
	}
	var pill string
	if s.kbFocused {
		pill = RenderPillUnderlined(label, state)
	} else {
		pill = RenderPill(label, state)
	}
	s.pillWidth = ansi.StringWidth(pill)
	return pill
}

func (s *StreamSelector) pillLabel() string {
	if s.current != "" {
		return s.current
	}
	if len(s.streams) == 0 {
		return "(none)"
	}
	return "(select)"
}

// SetOrigin records the pill's top-left in the final view. Called by parent
// after computing the render layout.
func (s *StreamSelector) SetOrigin(x, y int) {
	s.pillX = x
	s.pillY = y
}

// PillWidth returns the last-rendered pill width (rune-cells). Parent uses
// this to lay out neighbours (segment/toolbar) to the right of the pill.
func (s *StreamSelector) PillWidth() int { return s.pillWidth }

// RenderDropdown returns (fg, x, y, ok). ok=false when the dropdown should
// not be rendered (closed, or no streams). Parent should splice via
// components.PlaceOverlayAt(bg, fg, x, y).
func (s *StreamSelector) RenderDropdown() (string, int, int, bool) {
	if !s.open || len(s.streams) == 0 {
		return "", 0, 0, false
	}

	// Content width = enough for the longest stream name + a marker column,
	// clamped to a sane range so ridiculously long names don't blow up the
	// dropdown.
	const minInner = 16
	const maxInner = 40
	longest := 0
	for _, n := range s.streams {
		if w := ansi.StringWidth(n); w > longest {
			longest = w
		}
	}
	inner := longest + 3 // "▸ " prefix + trailing pad
	if inner < minInner {
		inner = minInner
	}
	if inner > maxInner {
		inner = maxInner
	}
	s.ddInnerWidth = inner

	rowStyleIdle := lipgloss.NewStyle().
		Foreground(ui.TextColor).
		Background(ui.BgLightColor).
		Width(inner)
	rowStyleActive := lipgloss.NewStyle().
		Foreground(ui.BrandPrimary).
		Background(ui.BgLighter).
		Bold(true).
		Width(inner)
	currentMarker := "▸ "
	blankMarker := "  "

	var rows []string
	for i, n := range s.streams {
		marker := blankMarker
		if n == s.current {
			marker = currentMarker
		}
		label := marker + truncateName(n, inner-2)
		if i == s.highlight {
			rows = append(rows, rowStyleActive.Render(label))
		} else {
			rows = append(rows, rowStyleIdle.Render(label))
		}
	}
	s.ddRowsRendered = len(rows)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.BrandPrimary).
		Background(ui.BgLightColor).
		Render(strings.Join(rows, "\n"))

	// Dropdown top edge sits just under the pill.
	x := s.pillX
	y := s.pillY + 1
	s.ddX = x
	s.ddY = y
	s.ddContentY = y + 1 // +1 for the top border row
	return box, x, y, true
}

func truncateName(name string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(name) <= width {
		return name
	}
	return ansi.Truncate(name, width-1, "") + "…"
}

// HandleKey processes a key while the dropdown is open. Returns:
//   - chosen: the picked stream name (empty if none picked)
//   - closed: true when the dropdown should close
//   - handled: true when the key was consumed (parent should not process it
//     further)
func (s *StreamSelector) HandleKey(msg tea.KeyMsg) (chosen string, closed, handled bool) {
	if !s.open {
		return "", false, false
	}
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if s.highlight > 0 {
			s.highlight--
		}
		return "", false, true
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if s.highlight < len(s.streams)-1 {
			s.highlight++
		}
		return "", false, true
	case key.Matches(msg, key.NewBinding(key.WithKeys("home", "g"))):
		s.highlight = 0
		return "", false, true
	case key.Matches(msg, key.NewBinding(key.WithKeys("end", "G"))):
		s.highlight = len(s.streams) - 1
		return "", false, true
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if s.highlight >= 0 && s.highlight < len(s.streams) {
			picked := s.streams[s.highlight]
			s.open = false
			return picked, true, true
		}
		s.open = false
		return "", true, true
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		s.open = false
		return "", true, true
	}
	// Swallow everything else while the dropdown owns focus.
	return "", false, true
}

// HandleMouse routes a mouse event. When the dropdown is open, clicks on a
// row select and close; clicks anywhere else close without picking. When
// closed, a click on the pill opens the dropdown.
func (s *StreamSelector) HandleMouse(msg tea.MouseMsg) (chosen string, closed, handled bool) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return "", false, false
	}
	if s.open {
		// Hit-test dropdown rows. Allow msg.Y to be reported one row above
		// the visible row (same ±1 tolerance used for the pill and other
		// widgets in this codebase).
		if msg.Y+1 >= s.ddContentY && msg.Y < s.ddContentY+s.ddRowsRendered &&
			msg.X >= s.ddX+1 && msg.X <= s.ddX+s.ddInnerWidth {
			idx := msg.Y - s.ddContentY
			if idx < 0 {
				idx = 0
			}
			if idx >= s.ddRowsRendered {
				idx = s.ddRowsRendered - 1
			}
			s.highlight = idx
			picked := s.streams[idx]
			s.open = false
			return picked, true, true
		}
		// Click outside → cancel.
		s.open = false
		return "", true, true
	}
	// Closed: clicking the pill opens. ±1 Y tolerance matches the codebase
	// convention (Tabs / Toolbar / segment all allow the mouse Y to arrive
	// one row above the visible row). X range is slightly widened to cover
	// the pill's Powerline caps on either end.
	if (msg.Y == s.pillY || msg.Y+1 == s.pillY) &&
		msg.X >= s.pillX-1 && msg.X <= s.pillX+s.pillWidth {
		s.Open()
		return "", false, true
	}
	return "", false, false
}
