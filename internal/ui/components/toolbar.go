package components

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/CooDdk/freexnats/internal/ui"
)

// ToolbarAction is one button in a Toolbar row.
// ID is returned by HandleMouse when the button is clicked; the page decides
// what to do with it (typically map to the same code path as a keyboard shortcut).
type ToolbarAction struct {
	ID       string
	Label    string
	Icon     string
	Disabled bool
	// Primary highlights the button in Primary color (e.g. the main "New" action).
	Primary bool
}

// Toolbar renders a horizontal row of clickable pill buttons.
//
// Hit-testing tolerates minor terminal-vs-runewidth mismatches by allowing a
// ±1 row window around the toolbar's absolute Y and a ±2 col slop on each
// button's X range. Buttons are separated by a 2-col gap so the slop cannot
// make adjacent buttons overlap.
type Toolbar struct {
	zonePrefix   string
	actions      []ToolbarAction
	topX         int
	topY         int
	buttonWidths []int
	pressedIdx   int
	// Focus state for keyboard navigation. focused=true means the toolbar
	// is the currently active chrome element; focusedIdx is the button
	// that receives Enter/HandleArrow.
	focused    bool
	focusedIdx int
}

func NewToolbar(zonePrefix string, actions []ToolbarAction) *Toolbar {
	return &Toolbar{zonePrefix: zonePrefix, actions: actions, pressedIdx: -1, focusedIdx: 0}
}

// SetActions replaces the button set. Use to update Disabled state (e.g. when
// the NATS connection drops).
func (t *Toolbar) SetActions(actions []ToolbarAction) {
	t.actions = actions
	t.buttonWidths = nil // stale until next View()
	t.pressedIdx = -1    // stale index against new button set
}

// SetDisabled toggles Disabled on a specific action by ID. No-op if not found.
func (t *Toolbar) SetDisabled(id string, disabled bool) {
	for i := range t.actions {
		if t.actions[i].ID == id {
			t.actions[i].Disabled = disabled
			return
		}
	}
}

// SetTopLeft records the absolute (x, y) of the toolbar's first rendered
// column and row in the final view. Once set (with y > 0), HandleMouse uses
// coordinate math instead of bubblezone.
func (t *Toolbar) SetTopLeft(x, y int) {
	t.topX = x
	t.topY = y
}

// HandleMouse returns the action ID under the mouse press, or ("", false) if
// no button was hit. Non-press events (release, motion) clear the transient
// "pressed" visual state so the flash naturally lasts from mouse-down to
// mouse-up.
func (t *Toolbar) HandleMouse(msg tea.MouseMsg) (string, bool) {
	// Any non-press mouse event clears the flash. This makes the pressed look
	// appear roughly for the duration the button is held, without needing a
	// tea.Tick + per-page message handler.
	if msg.Action != tea.MouseActionPress {
		if t.pressedIdx >= 0 {
			t.pressedIdx = -1
		}
		return "", false
	}
	if msg.Button != tea.MouseButtonLeft {
		return "", false
	}
	idx := t.hitTest(msg)
	if idx < 0 {
		return "", false
	}
	t.pressedIdx = idx
	return t.actions[idx].ID, true
}

// hitTest returns the button index under the mouse press, or -1. Tolerates
// ±1 row on Y (some terminals report mouse.Y one row less than the visible
// row) and ±2 cols on X (small runewidth/glyph-metric mismatches).
//
// Once SetTopLeft has landed (topY > 0), coordinate math is authoritative:
// a click whose Y is outside the toolbar row returns -1 immediately, without
// falling through to the zone fallback. Otherwise zone marks — whose recorded
// positions can be perturbed by outer lipgloss composition — would "grab"
// clicks that visually belong to widgets rendered above the toolbar (e.g.
// the top tab bar), causing overlapping hit regions.
func (t *Toolbar) hitTest(msg tea.MouseMsg) int {
	if t.topY > 0 && len(t.buttonWidths) == len(t.actions) {
		if msg.Y != t.topY && msg.Y+1 != t.topY {
			return -1
		}
		cur := t.topX + 2 // leading "  " prefix in View()
		for i, a := range t.actions {
			w := t.buttonWidths[i]
			if !a.Disabled && msg.X >= cur-2 && msg.X <= cur+w+1 {
				return i
			}
			cur += w + 2 // "  " separator between buttons
		}
		return -1
	}
	// Initial-frame fallback: topY hasn't been set yet, use zone marks.
	for i, a := range t.actions {
		if a.Disabled {
			continue
		}
		if zone.Get(t.zoneID(i)).InBounds(msg) {
			return i
		}
	}
	return -1
}

// Height is always 1 row (the button row). Pages should reserve one extra
// blank line below when composing layouts.
func (t *Toolbar) Height() int { return 1 }

func (t *Toolbar) View() string {
	if len(t.actions) == 0 {
		t.buttonWidths = nil
		return ""
	}
	parts := make([]string, 0, len(t.actions))
	widths := make([]int, 0, len(t.actions))
	for i, a := range t.actions {
		rendered := t.renderButton(i, a)
		parts = append(parts, rendered)
		widths = append(widths, lipgloss.Width(rendered))
	}
	t.buttonWidths = widths
	return "  " + strings.Join(parts, "  ")
}

// Rounded pill caps. U+E0B6/U+E0B4 are Nerd Font "powerline extra" half-circles;
// rendered with the button's background as foreground (and no background) they
// blend into the surrounding surface to give the button a pill shape.
const (
	capLeft  = ""
	capRight = ""
)

func (t *Toolbar) renderButton(idx int, a ToolbarAction) string {
	label := a.Label
	if a.Icon != "" {
		label = a.Icon + " " + label
	}

	pressed := idx == t.pressedIdx
	kbFocused := t.focused && idx == t.focusedIdx

	var fg, bg lipgloss.TerminalColor
	bold := true
	switch {
	case a.Disabled:
		fg = ui.TextFaint
		bg = ui.BgLightColor
		bold = false
	case pressed:
		// Pressed: brand accent flash regardless of Primary flag.
		fg = ui.SelectionFg
		bg = ui.BrandPrimary
	case kbFocused:
		// Keyboard-focused: light up in Primary color so focus is
		// visually unambiguous. Primary and non-Primary buttons share
		// the same focused look — the Primary flag no longer affects
		// the idle rendering (Phase 1 design: uniform idle, focus is
		// the only highlight).
		fg = ui.SelectionFg
		bg = ui.Primary
	default:
		// Idle: subtle accent bg for every button, Primary or not.
		fg = ui.TextColor
		bg = ui.BgLighter
	}

	rendered := RenderPillWithColors(label, fg, bg, bold)

	if a.Disabled {
		return rendered
	}
	return zone.Mark(t.zoneID(idx), rendered)
}

func (t *Toolbar) zoneID(idx int) string {
	return t.zonePrefix + "-btn-" + strconv.Itoa(idx)
}

// --- Keyboard focus API used by the focus.Manager ---------------------------

// Focus / Blur toggle the keyboard-focus visual for the currently focused
// button (see focusedIdx). Blurring does not reset focusedIdx so a user
// leaving and re-entering the toolbar via arrow keys lands back on the
// last-used button.
func (t *Toolbar) Focus()      { t.focused = true }
func (t *Toolbar) Blur()       { t.focused = false }
func (t *Toolbar) IsFocused() bool { return t.focused }

// FocusedIdx returns the currently keyboard-focused button index.
func (t *Toolbar) FocusedIdx() int { return t.focusedIdx }

// SetFocusedIdx moves the keyboard focus to a specific button. Clamps to
// valid range; skips over Disabled buttons if possible.
func (t *Toolbar) SetFocusedIdx(idx int) {
	if len(t.actions) == 0 {
		t.focusedIdx = 0
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(t.actions) {
		idx = len(t.actions) - 1
	}
	t.focusedIdx = idx
}

// FocusFirstEnabled sets focus to the first non-Disabled button, or 0 if
// none are enabled. Used when the toolbar becomes focused via ↓ from Tabs.
func (t *Toolbar) FocusFirstEnabled() {
	for i, a := range t.actions {
		if !a.Disabled {
			t.focusedIdx = i
			return
		}
	}
	t.focusedIdx = 0
}

// StepFocus walks the keyboard focus by delta (+1 right / -1 left),
// skipping Disabled buttons. Returns true if the step landed on a new
// button; false if the toolbar is at its edge in that direction.
func (t *Toolbar) StepFocus(delta int) bool {
	if len(t.actions) == 0 {
		return false
	}
	i := t.focusedIdx + delta
	for i >= 0 && i < len(t.actions) {
		if !t.actions[i].Disabled {
			t.focusedIdx = i
			return true
		}
		i += delta
	}
	return false
}

// ActionAtFocus returns the currently keyboard-focused action, or an empty
// ToolbarAction (ID=="") if none. Used by focus adapters to fire the same
// codepath as a mouse click.
func (t *Toolbar) ActionAtFocus() ToolbarAction {
	if t.focusedIdx < 0 || t.focusedIdx >= len(t.actions) {
		return ToolbarAction{}
	}
	return t.actions[t.focusedIdx]
}

