// Package focus provides a small keyboard-focus manager for the app's
// chrome-level widgets. Pages build a linear list of Items on entry; the
// manager routes arrow keys and Enter to the currently focused Item, and
// escapes ("at edge") events walk to the previous/next Item.
//
// Up/Left walk backward in the list; Down/Right walk forward. Widgets
// that don't want horizontal escape (Toolbar mid-cursor, TableList) return
// handled=true for Left/Right so they never trigger the walk; widgets that
// do (Segment control at its edge, Toolbar at leftmost/rightmost button)
// return false and get walked to their neighbor.
package focus

import tea "github.com/charmbracelet/bubbletea"

// JumpMsg is emitted by an Item that wants to hand focus to a specific index
// in the current list instead of relying on linear escape. The parent app
// catches it and calls Manager.SetCurrent(Target). Use focus.Jump(target)
// to build a cmd that dispatches it.
//
// This is the escape hatch for 2D layouts: when a widget on the top row wants
// ↓ to skip the same-row neighbours and land on a widget below, linear walking
// isn't enough. The widget returns handled=true plus focus.Jump(idx).
type JumpMsg struct{ Target int }

func Jump(target int) tea.Cmd {
	return func() tea.Msg { return JumpMsg{Target: target} }
}

type Direction int

const (
	DirUp Direction = iota
	DirDown
	DirLeft
	DirRight
)

// Item is one focusable region on a page. Implementations own their own
// focus state and rendering; the manager only calls into these methods.
type Item interface {
	// Focus / Blur flip the visual focus indicator.
	Focus()
	Blur()

	// HandleArrow returns (cmd, handled). handled=false means the Item
	// is at the edge in that direction and the manager should escape
	// to the previous/next Item.
	HandleArrow(dir Direction) (tea.Cmd, bool)

	// Activate is called on Enter. Return nil if there's nothing to do.
	Activate() tea.Cmd
}

// Manager owns the linear focus list for the current page. Rebuild on
// page change / mode change via SetItems.
type Manager struct {
	items []Item
	cur   int
}

func NewManager() *Manager { return &Manager{cur: -1} }

// SetItems replaces the focus list. The first item is focused by default;
// pass want=-1 to leave nothing focused (rare — most pages want a default).
func (m *Manager) SetItems(items []Item, want int) {
	if m.cur >= 0 && m.cur < len(m.items) {
		m.items[m.cur].Blur()
	}
	m.items = items
	m.cur = -1
	if len(items) == 0 {
		return
	}
	if want < 0 || want >= len(items) {
		want = 0
	}
	m.cur = want
	items[want].Focus()
}

// Clear removes all items and blurs anything focused.
func (m *Manager) Clear() {
	if m.cur >= 0 && m.cur < len(m.items) {
		m.items[m.cur].Blur()
	}
	m.items = nil
	m.cur = -1
}

// Current returns the currently focused item index, or -1.
func (m *Manager) Current() int { return m.cur }

// SetCurrent focuses the item at idx (blurring the previous one). No-op on
// out-of-range.
func (m *Manager) SetCurrent(idx int) {
	if idx < 0 || idx >= len(m.items) {
		return
	}
	if m.cur == idx {
		return
	}
	if m.cur >= 0 && m.cur < len(m.items) {
		m.items[m.cur].Blur()
	}
	m.cur = idx
	m.items[idx].Focus()
}

// HandleKey routes an arrow/Enter key to the focused item. Returns
// (cmd, handled). handled=true means the manager (or the item) consumed
// the key and the caller should stop dispatching.
func (m *Manager) HandleKey(key string) (tea.Cmd, bool) {
	if m.cur < 0 || m.cur >= len(m.items) {
		return nil, false
	}
	if key == "enter" {
		return m.items[m.cur].Activate(), true
	}
	var dir Direction
	switch key {
	case "up":
		dir = DirUp
	case "down":
		dir = DirDown
	case "left":
		dir = DirLeft
	case "right":
		dir = DirRight
	default:
		return nil, false
	}
	cmd, handled := m.items[m.cur].HandleArrow(dir)
	if handled {
		return cmd, true
	}
	// Escape: walk the linear list. Up/Left step back, Down/Right step
	// forward. Widgets that don't want horizontal escape (Toolbar, Table)
	// return handled=true for Left/Right so they never reach this branch;
	// widgets that do (Segment control at its edge) return false and get
	// walked to their neighbor here.
	switch dir {
	case DirUp, DirLeft:
		if m.cur > 0 {
			m.SetCurrent(m.cur - 1)
		}
	case DirDown, DirRight:
		if m.cur < len(m.items)-1 {
			m.SetCurrent(m.cur + 1)
		}
	}
	// Absorb the key either way so it doesn't fall through to page-level
	// handlers and double-fire (e.g. Streams' j/k rebinding).
	return nil, true
}
