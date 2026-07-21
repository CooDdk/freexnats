package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/ui"
)

// ButtonState is the visual state a pill button can be in.
type ButtonState int

const (
	// ButtonIdle is the muted default: unfocused, unselected buttons.
	ButtonIdle ButtonState = iota
	// ButtonFocused signals keyboard focus OR "currently selected" for
	// segmented controls / tabs — visually the Primary accent.
	ButtonFocused
	// ButtonWarning signals a state change (e.g. tail "Resume" when paused).
	ButtonWarning
	// ButtonDanger signals a destructive action.
	ButtonDanger
	// ButtonDisabled: greyed out, non-interactive.
	ButtonDisabled
)

// RenderPill renders a rounded pill button using Powerline half-circle caps
// (capLeft/capRight declared in toolbar.go). Every pill button/segment/tab in
// the app should route through here so:
//   - Unfocused buttons across every form/dialog/tail control share the exact
//     same muted look. Only the focused button lights up.
//   - Widths stay in lock-step across state changes — a focus swap only
//     recolors, no layout shift.
func RenderPill(label string, state ButtonState) string {
	fg, bg, bold := pillColors(state)
	return RenderPillWithColors(label, fg, bg, bold)
}

// RenderPillWithColors is the low-level variant for widgets like Toolbar that
// maintain their own state ladder beyond ButtonState (idle/focused/pressed/
// primary-variant/disabled). Callers pick the fg/bg; the pill shape stays
// identical to RenderPill so mixed-source pills line up.
func RenderPillWithColors(label string, fg, bg lipgloss.TerminalColor, bold bool) string {
	return renderPill(label, fg, bg, bold, false)
}

// RenderPillUnderlined is RenderPill with an underline decoration on the body
// text. Used for the segment control's keyboard-focus indicator: the pill's
// primary/idle color still signals which side is active, and the underline
// signals "keyboard focus lives here." Applying underline via the body's own
// lipgloss style (rather than pre-wrapping the label in an ANSI escape) keeps
// the body/cap ANSI boundary clean so the pill shape renders as one unit.
func RenderPillUnderlined(label string, state ButtonState) string {
	fg, bg, bold := pillColors(state)
	return renderPill(label, fg, bg, bold, true)
}

func pillColors(state ButtonState) (lipgloss.TerminalColor, lipgloss.TerminalColor, bool) {
	var fg, bg lipgloss.TerminalColor
	bold := true
	switch state {
	case ButtonFocused:
		fg, bg = ui.SelectionFg, ui.Primary
	case ButtonWarning:
		fg, bg = ui.SelectionFg, ui.Warning
	case ButtonDanger:
		fg, bg = ui.SelectionFg, ui.Error
	case ButtonDisabled:
		fg, bg = ui.TextFaint, ui.BgLightColor
		bold = false
	default:
		fg, bg = ui.TextMuted, ui.BgLighter
	}
	return fg, bg, bold
}

func renderPill(label string, fg, bg lipgloss.TerminalColor, bold, underline bool) string {
	inner := " " + label + " "
	body := lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Bold(bold).
		Underline(underline).
		Render(inner)
	capStyle := lipgloss.NewStyle().Foreground(bg)
	return capStyle.Render(capLeft) + body + capStyle.Render(capRight)
}

// pillState is a convenience for the common Idle/Focused toggle used by forms
// and segmented controls.
func pillState(focused bool) ButtonState {
	if focused {
		return ButtonFocused
	}
	return ButtonIdle
}
