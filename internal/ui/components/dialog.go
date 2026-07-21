package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/ui"
)

type DialogType int

const (
	DialogInfo DialogType = iota
	DialogConfirm
	DialogError
	DialogSuccess
)

type Dialog struct {
	title     string
	message   string
	dialogType DialogType
	visible   bool
	buttons   []string
	selected  int
	onConfirm func() tea.Msg
	onCancel  func() tea.Msg
}

func NewDialog() *Dialog {
	return &Dialog{
		buttons:  []string{"OK"},
		selected: 0,
	}
}

func (d *Dialog) Show(title, message string, dialogType DialogType) {
	d.title = title
	d.message = message
	d.dialogType = dialogType
	d.visible = true
	d.selected = 0

	switch dialogType {
	case DialogConfirm:
		d.buttons = []string{"Yes", "No"}
	default:
		d.buttons = []string{"OK"}
	}
}

func (d *Dialog) Hide() {
	d.visible = false
}

func (d *Dialog) Visible() bool {
	return d.visible
}

func (d *Dialog) Update(msg tea.Msg) (*Dialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if d.selected > 0 {
				d.selected--
			}
		case "right", "l":
			if d.selected < len(d.buttons)-1 {
				d.selected++
			}
		case "enter":
			d.visible = false
			if d.selected == 0 {
				return d, func() tea.Msg { return DialogConfirmMsg{} }
			}
			return d, func() tea.Msg { return DialogCancelMsg{} }
		case "esc":
			d.visible = false
			return d, func() tea.Msg { return DialogCancelMsg{} }
		}
	}

	return d, nil
}

type DialogConfirmMsg struct{}
type DialogCancelMsg struct{}

func (d *Dialog) View(width, height int) string {
	if !d.visible {
		return ""
	}

	dialogWidth := min(width-4, 60)
	contentWidth := dialogWidth - 4

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(d.titleColor()).
		Width(contentWidth).
		Align(lipgloss.Center)

	messageStyle := lipgloss.NewStyle().
		Foreground(ui.TextColor).
		Width(contentWidth).
		Align(lipgloss.Center)

	wrappedMsg := wrapText(d.message, contentWidth-2)
	messageLines := strings.Split(wrappedMsg, "\n")

	var buttons []string
	for i, btn := range d.buttons {
		state := ButtonIdle
		if i == d.selected {
			// Yes on a Confirm dialog is destructive-adjacent; keep Focused
			// (Primary) for now — dialog-level destructive coloring is handled
			// by border color already.
			state = ButtonFocused
		}
		buttons = append(buttons, RenderPill(btn, state))
	}
	buttonRow := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Center).
		Render(strings.Join(buttons, "  "))

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(d.borderColor()).
		Background(ui.BgLightColor).
		Padding(1, 2).
		Width(dialogWidth)

	content := lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render(d.title),
		"",
		messageStyle.Render(strings.Join(messageLines, "\n")),
		"",
		buttonRow,
	)

	dialog := borderStyle.Render(content)

	dialogHeight := lipgloss.Height(dialog)
	topPad := (height - dialogHeight) / 2
	if topPad < 0 {
		topPad = 0
	}

	overlay := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#000000")).
		Render("")

	_ = overlay

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog)
}

func (d *Dialog) titleColor() lipgloss.Color {
	switch d.dialogType {
	case DialogError:
		return ui.Error
	case DialogSuccess:
		return ui.Success
	case DialogConfirm:
		return ui.Warning
	default:
		return ui.Primary
	}
}

func (d *Dialog) borderColor() lipgloss.Color {
	switch d.dialogType {
	case DialogError:
		return ui.Error
	case DialogSuccess:
		return ui.Success
	case DialogConfirm:
		return ui.Warning
	default:
		return ui.BorderColor
	}
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	if len(text) <= width {
		return text
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)

	return strings.Join(lines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
