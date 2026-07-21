package components

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// Tabs renders a horizontal tab strip. Selected tab is a rounded pill in
// Primary color; unselected tabs are flat muted labels. Hit-testing mirrors
// Toolbar's approach: coordinate-based with ±1 row Y and ±2 col X tolerance,
// with a bubblezone fallback for the first frame before SetTopLeft lands.
type Tabs struct {
	zonePrefix string
	items      []TabItem
	selected   int
	topX       int
	topY       int
	itemWidths []int
	// focused mirrors the keyboard-focus state (managed by focus.Manager).
	// Tabs' focused idx and selected idx are always the same thing (moving
	// focus IS moving selection), so the visual doesn't need to change on
	// blur — the user tells Tabs has focus because the toolbar/table below
	// dim their own selections.
	focused bool
}

// Focus / Blur toggle keyboard-focus state. See the struct comment: no
// visual change on Tabs itself, but the flag is still tracked for
// completeness and future spatial-focus resolution.
func (t *Tabs) Focus() { t.focused = true }
func (t *Tabs) Blur()  { t.focused = false }

type TabItem struct {
	ID   string
	Icon string
	Name string
}

func NewTabs(zonePrefix string, items []TabItem) *Tabs {
	return &Tabs{zonePrefix: zonePrefix, items: items}
}

func (t *Tabs) SetTopLeft(x, y int) {
	t.topX = x
	t.topY = y
}

func (t *Tabs) Selected() int { return t.selected }

func (t *Tabs) SetSelected(idx int) {
	if idx >= 0 && idx < len(t.items) {
		t.selected = idx
	}
}

func (t *Tabs) Count() int { return len(t.items) }

// TopY returns the absolute Y row where the tab strip is rendered, or 0 if
// SetTopLeft hasn't been called yet (i.e. before the first View()). Callers
// use this to route mouse events by Y region so the tab strip never sees
// clicks that land below it.
func (t *Tabs) TopY() int { return t.topY }

func (t *Tabs) Next() {
	if len(t.items) == 0 {
		return
	}
	t.selected = (t.selected + 1) % len(t.items)
}

func (t *Tabs) Prev() {
	if len(t.items) == 0 {
		return
	}
	t.selected = (t.selected - 1 + len(t.items)) % len(t.items)
}

// HitTest returns the tab index under the mouse press, or -1 if none.
// Non-press events are ignored (unlike Toolbar we don't track pressed state
// visually — the tab-switch itself is instant feedback).
//
// Once SetTopLeft has landed (topY > 0), coordinate math is authoritative:
// a click whose Y is outside the tab row returns -1 immediately, without
// falling through to bubblezone. The zone fallback would otherwise "grab"
// clicks that visually belong to widgets rendered below the tab bar (e.g.
// the toolbar's Publish button), because outer lipgloss composition can
// perturb the zone marks' recorded positions.
func (t *Tabs) HitTest(msg tea.MouseMsg) int {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return -1
	}
	if t.topY > 0 && len(t.itemWidths) == len(t.items) {
		if msg.Y != t.topY && msg.Y+1 != t.topY {
			return -1
		}
		cur := t.topX + 2 // leading "  " prefix in View()
		for i := range t.items {
			w := t.itemWidths[i]
			if msg.X >= cur-2 && msg.X <= cur+w+1 {
				return i
			}
			cur += w + 2 // "  " separator between tabs
		}
		return -1
	}
	// Initial-frame fallback: topY hasn't been set yet, use zone marks.
	for i := range t.items {
		if zone.Get(t.zoneID(i)).InBounds(msg) {
			return i
		}
	}
	return -1
}

// Height is always 1 row. Callers should reserve one blank line below when
// composing layouts.
func (t *Tabs) Height() int { return 1 }

func (t *Tabs) View() string {
	if len(t.items) == 0 {
		t.itemWidths = nil
		return ""
	}
	parts := make([]string, 0, len(t.items))
	widths := make([]int, 0, len(t.items))
	for i, it := range t.items {
		rendered := t.renderTab(i, it)
		parts = append(parts, rendered)
		widths = append(widths, lipgloss.Width(rendered))
	}
	t.itemWidths = widths
	return "  " + strings.Join(parts, "  ")
}

func (t *Tabs) renderTab(idx int, it TabItem) string {
	label := it.Name
	if it.Icon != "" {
		label = it.Icon + " " + label
	}
	state := ButtonIdle
	if idx == t.selected {
		state = ButtonFocused
	}
	return zone.Mark(t.zoneID(idx), RenderPill(label, state))
}

func (t *Tabs) zoneID(idx int) string {
	return t.zonePrefix + "-tab-" + strconv.Itoa(idx)
}
