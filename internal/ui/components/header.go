package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/config"
	"github.com/CooDdk/freexnats/internal/ui"
)

type HeaderBar struct {
	width       int
	serverURL   string
	connected   bool
	connecting  bool
}

func NewHeaderBar() *HeaderBar {
	return &HeaderBar{}
}

func (h *HeaderBar) SetWidth(width int) {
	h.width = width
}

func (h *HeaderBar) SetConnection(url string, connected bool, connecting bool) {
	h.serverURL = url
	h.connected = connected
	h.connecting = connecting
}

func (h *HeaderBar) View() string {
	left := h.renderLeft()
	right := h.renderRight()

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	middleWidth := h.width - leftWidth - rightWidth
	if middleWidth < 1 {
		middleWidth = 1
	}

	middle := strings.Repeat(" ", middleWidth)

	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#181825")).
		Foreground(ui.TextColor).
		Padding(0, 1)

	fullContent := left + middle + right
	return barStyle.Width(h.width).Render(fullContent)
}

func (h *HeaderBar) renderLeft() string {
	logo := lipgloss.NewStyle().
		Foreground(ui.BrandPrimary).
		Bold(true).
		Render("⚡ ")

	name := lipgloss.NewStyle().
		Foreground(ui.BrandPrimary).
		Bold(true).
		Render(config.AppName)

	version := lipgloss.NewStyle().
		Foreground(ui.SubtleColor).
		Render(" " + config.AppVersion)

	return logo + name + version
}

func (h *HeaderBar) renderRight() string {
	var statusDot string
	var statusText string

	switch {
	case h.connecting:
		statusDot = lipgloss.NewStyle().
			Foreground(ui.WarningColor).
			Render("●")
		statusText = lipgloss.NewStyle().
			Foreground(ui.WarningColor).
			Render("Connecting")
	case h.connected:
		statusDot = lipgloss.NewStyle().
			Foreground(ui.SuccessColor).
			Render("●")
		statusText = lipgloss.NewStyle().
			Foreground(ui.SuccessColor).
			Render("Connected")
	default:
		statusDot = lipgloss.NewStyle().
			Foreground(ui.ErrorColor).
			Render("●")
		statusText = lipgloss.NewStyle().
			Foreground(ui.ErrorColor).
			Render("Disconnected")
	}

	url := lipgloss.NewStyle().
		Foreground(ui.SubtleColor).
		Render(h.serverURL)

	return statusDot + " " + statusText + "  " + url + " "
}
