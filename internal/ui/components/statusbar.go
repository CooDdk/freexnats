package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/CooDdk/freexnats/internal/ui"
)

type StatusBar struct {
	width int
	left  string
	right string
}

func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

func (s *StatusBar) SetWidth(width int) {
	s.width = width
}

func (s *StatusBar) SetContent(left, right string) {
	s.left = left
	s.right = right
}

func (s *StatusBar) View() string {
	divider := lipgloss.NewStyle().
		Foreground(ui.BgLighter).
		Render(strings.Repeat("\u2500", s.width))

	leftStyle := lipgloss.NewStyle().
		Foreground(ui.TextColor).
		Background(ui.BgLightColor).
		Padding(0, 2)

	rightStyle := lipgloss.NewStyle().
		Foreground(ui.TextFaint).
		Background(ui.BgLightColor).
		Align(lipgloss.Right).
		Padding(0, 2)

	// Guarantee a single-row bar: if left + right + minimum gap can't fit in
	// s.width, truncate the right (help) text with an ellipsis. Wrapping would
	// silently add a row and offset the whole outer layout \u2014 see the Messages
	// help-text bug where a long hint pushed the LOGO out of the terminal.
	const padCols = 4 // leftStyle + rightStyle each add Padding(0,2) = 4 cols
	const minGap = 2
	leftDisplay := runewidth.StringWidth(s.left)
	rightDisplay := runewidth.StringWidth(s.right)
	leftWidth := leftDisplay + padCols
	rightWidth := rightDisplay + padCols
	right := s.right
	if leftWidth+rightWidth+minGap > s.width {
		budget := s.width - leftWidth - padCols - minGap
		if budget < 1 {
			budget = 1
		}
		right = runewidth.Truncate(s.right, budget, "\u2026")
		rightWidth = runewidth.StringWidth(right) + padCols
	}

	middleWidth := s.width - leftWidth - rightWidth
	if middleWidth < 0 {
		middleWidth = 0
	}
	middle := strings.Repeat(" ", middleWidth)

	bar := lipgloss.NewStyle().
		Background(ui.BgLightColor).
		Width(s.width).
		Render(
			leftStyle.Render(s.left) +
				middle +
				rightStyle.Render(right),
		)

	return divider + "\n" + bar
}
