package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/config"
	"github.com/CooDdk/freexnats/internal/ui"
)

const brandLabel = "FreeX Nats"

var freexLogoRows = []string{
	"FFFFFF RRRR   EEEEEE EEEEEE X    X",
	"FF     RR RR  EE     EE      X  X ",
	"FFFF   RRRR   EEEE   EEEE     XX  ",
	"FF     RR RR  EE     EE      X  X ",
	"FF     RR  RR EEEEEE EEEEEE X    X",
}

var natsLogoRows = []string{
	"NN   N   AAA   TTTTT  SSSSS",
	"NNN  N  AA AA    T   SS    ",
	"N N  N AA   AA   T    SSSS ",
	"N  NNN AAAAAAA   T       SS",
	"N   NN AA   AA   T   SSSSS ",
}

func PixelLogo() string {
	rows := make([]string, 0, len(freexLogoRows))
	freexStyle := lipgloss.NewStyle().Foreground(ui.LogoColor1).Bold(true)
	natsStyle := lipgloss.NewStyle().Foreground(ui.LogoColor3).Bold(true)

	for i := range freexLogoRows {
		rows = append(rows, freexStyle.Render(freexLogoRows[i])+"      "+natsStyle.Render(natsLogoRows[i]))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func BrandTitle() string {
	freeStyle := lipgloss.NewStyle().Foreground(ui.LogoColor1).Bold(true)
	natsStyle := lipgloss.NewStyle().Foreground(ui.LogoColor3).Bold(true)

	return freeStyle.Render("FreeX") + " " + natsStyle.Render("Nats")
}

func decorativeRule(width int) string {
	if width < 24 {
		width = 24
	}

	return lipgloss.NewStyle().
		Foreground(ui.LogoTrim).
		Faint(true).
		Render(strings.Repeat("=", width))
}

func LogoWithSubtitle() string {
	logo := PixelLogo()
	rule := decorativeRule(maxLineWidth(logo))

	brandLine := lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().Foreground(ui.LogoTrimDim).Render("\u2500\u2500  "),
		BrandTitle(),
		lipgloss.NewStyle().Foreground(ui.LogoTrimDim).Render("  \u00b7  "),
		lipgloss.NewStyle().Foreground(ui.BrandAccent).Bold(true).Render(config.AppVersion),
		lipgloss.NewStyle().Foreground(ui.LogoTrimDim).Render(" \u2500\u2500"),
	)

	descLine := lipgloss.NewStyle().
		Foreground(ui.TextFaint).
		Render(config.AppDesc)

	return lipgloss.JoinVertical(
		lipgloss.Center,
		rule,
		logo,
		rule,
		"",
		brandLine,
		descLine,
	)
}

func PixelLogoHeight() int {
	return len(freexLogoRows) + 6
}

func maxLineWidth(input string) int {
	maxWidth := 0
	for _, line := range strings.Split(input, "\n") {
		width := lipgloss.Width(line)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}
