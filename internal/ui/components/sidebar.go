package components

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/CooDdk/freexnats/internal/ui"
)

const sidebarZonePrefix = "sidebar-item-"

type Sidebar struct {
	items       []SidebarItem
	selectedIdx int
	width       int
	topY        int
}

type SidebarItem struct {
	Icon string
	Name string
}

func NewSidebar(items []SidebarItem) *Sidebar {
	return &Sidebar{
		items: items,
	}
}

func (s *Sidebar) SetWidth(width int) {
	s.width = width
}

// SetTopY records the absolute y coordinate of the sidebar block's top row
// in the full rendered view. HitTest uses this for coordinate-based hits,
// which are more reliable than bubblezone marks when outer lipgloss styles
// (Border/Width) re-process the string and can shift or drop mark sequences.
func (s *Sidebar) SetTopY(y int) {
	s.topY = y
}

// Sidebar internal layout, relative to the block's top row:
//
//	rel y 0 : outer Padding(1, 0) top blank
//	rel y 1 : "NAVIGATE" header
//	rel y 2 : blank
//	rel y 3 : item[0]
//	rel y 4 : blank
//	rel y 5 : item[1]
//	...
const sidebarItemsRelY = 3

func (s *Sidebar) Selected() int {
	return s.selectedIdx
}

func (s *Sidebar) SetSelected(idx int) {
	if idx >= 0 && idx < len(s.items) {
		s.selectedIdx = idx
	}
}

func (s *Sidebar) Next() {
	s.selectedIdx = (s.selectedIdx + 1) % len(s.items)
}

func (s *Sidebar) Prev() {
	s.selectedIdx = (s.selectedIdx - 1 + len(s.items)) % len(s.items)
}

func (s *Sidebar) ItemCount() int {
	return len(s.items)
}

// HitTest returns the index of the item under the mouse event, or -1 if none.
// It prefers coordinate-based hit testing (using SetTopY + fixed layout)
// because bubblezone marks can be perturbed by outer lipgloss processing.
// The zone-based check is kept only as a fallback for the initial frame
// where topY has not been set yet.
func (s *Sidebar) HitTest(msg tea.MouseMsg) int {
	if s.width > 0 && msg.X >= 0 && msg.X < s.width {
		itemsStart := s.topY + sidebarItemsRelY
		// On this terminal, mouse.Y is reported one row less than the
		// visible row (Windows alt-screen + MouseCellMotion), so a click
		// on the visible item text lands on msg.Y = visual - 1. Shifting
		// by +1 maps it back to the visible row before the /2 division;
		// on terminals with no offset the shift lands on the blank line
		// below the item, which is still mapped to the same item.
		y := msg.Y + 1
		if y >= itemsStart {
			idx := (y - itemsStart) / 2
			if idx >= 0 && idx < len(s.items) {
				return idx
			}
		}
	}
	for i := range s.items {
		if zone.Get(sidebarZoneID(i)).InBounds(msg) {
			return i
		}
	}
	return -1
}

func (s *Sidebar) View() string {
	headerStyle := lipgloss.NewStyle().
		Foreground(ui.TextFaint).
		Bold(true).
		Padding(0, 2)

	var lines []string
	lines = append(lines, headerStyle.Render("NAVIGATE"))
	lines = append(lines, "")

	for i, item := range s.items {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s.renderItem(i, item))
	}

	lines = append(lines, "", "")

	hintStyle := lipgloss.NewStyle().
		Foreground(ui.TextFaint).
		Padding(0, 2)
	lines = append(lines, hintStyle.Render("Tab  Cycle"))

	return lipgloss.NewStyle().
		Padding(1, 0).
		Render(strings.Join(lines, "\n"))
}

func (s *Sidebar) renderItem(idx int, item SidebarItem) string {
	rowW := s.width - 2
	if rowW < 12 {
		rowW = 12
	}
	isSelected := idx == s.selectedIdx

	var accent, label string
	if isSelected {
		accent = lipgloss.NewStyle().Foreground(ui.Primary).Bold(true).Render("▎")
		label = lipgloss.NewStyle().Foreground(ui.SelectionFg).Bold(true).Render(item.Icon + " " + item.Name)
	} else {
		accent = " "
		label = lipgloss.NewStyle().Foreground(ui.TextMuted).Render(" " + item.Icon + " " + item.Name)
	}

	row := lipgloss.NewStyle().Width(rowW).Render(accent + label)
	item2 := lipgloss.NewStyle().Padding(0, 1).Render(row)
	return zone.Mark(sidebarZoneID(idx), item2)
}

func sidebarZoneID(idx int) string {
	return sidebarZonePrefix + strconv.Itoa(idx)
}
