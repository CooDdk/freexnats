package pages

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
)

// PublishForm is a self-contained modal for publishing a JetStream message.
// Text handling is delegated to bubbles/textinput and bubbles/textarea so
// keyboard editing (cursor movement, home/end, copy/paste) works natively.
// Mouse hit-testing uses coordinate math (like Toolbar) — zone marks are
// unreliable when the buttons sit inside a lipgloss border+padding wrapper.
type PublishForm struct {
	stream string

	subject textinput.Model
	headers textinput.Model
	payload textarea.Model
	field   int // pfFieldSubject..pfFieldSubmit

	err     string
	loading bool
	success bool
	lastSeq uint64

	width int
	// height is the vertical budget for the whole panel including borders and
	// padding. Set by MessagesPage.SetSize; when >0 the payload textarea is
	// shrunk so that title + inputs + buttons all fit inside the budget.
	// height ≤ 0 keeps the legacy 5-row textarea.
	height int

	// Panel top-left in the final view. Set by MessagesPage.SetOrigin.
	topX int
	topY int

	// Recorded during View() so HandleMouse can do coordinate math without
	// re-rendering. Absolute Y and X ranges into the final view.
	subjectY               int
	headersY               int
	payloadYStart          int
	payloadYEnd            int
	inputContentX          int
	buttonRowY             int
	cancelXStart, cancelXEnd int
	submitXStart, submitXEnd int
}

const (
	pfFieldSubject = 0
	pfFieldHeaders = 1
	pfFieldPayload = 2
	pfFieldCancel  = 3
	pfFieldSubmit  = 4
	pfFieldCount   = 5
)

type PublishResultMsg struct {
	Seq uint64
	Err error
}

func NewPublishForm(stream, defaultSubject string) *PublishForm {
	subj := textinput.New()
	subj.Prompt = ""
	subj.Placeholder = "e.g. orders.new"
	subj.SetValue(defaultSubject)
	subj.CursorEnd()
	applyInputStyles(&subj)
	subj.Focus()

	hdr := textinput.New()
	hdr.Prompt = ""
	hdr.Placeholder = "k1=v1, k2=v2 (optional)"
	applyInputStyles(&hdr)

	pay := textarea.New()
	pay.Prompt = ""
	pay.Placeholder = "hello world (any bytes; JSON/text/... — NATS doesn't parse it)"
	pay.ShowLineNumbers = false
	pay.SetHeight(5)
	applyTextareaStyles(&pay)

	return &PublishForm{
		stream:  stream,
		subject: subj,
		headers: hdr,
		payload: pay,
		field:   pfFieldSubject,
	}
}

func applyInputStyles(ti *textinput.Model) {
	ti.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(ui.SelectionFg).Background(ui.Primary)
	ti.Cursor.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)
}

func applyTextareaStyles(ta *textarea.Model) {
	ta.FocusedStyle.Base = lipgloss.NewStyle().Background(ui.BgLighter)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(ui.TextColor).Background(ui.BgLighter)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true).Background(ui.BgLighter)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(ui.BgLighter)
	ta.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(ui.BgLighter)

	ta.BlurredStyle.Base = lipgloss.NewStyle().Background(ui.BgLightColor)
	ta.BlurredStyle.Text = lipgloss.NewStyle().Foreground(ui.TextColor).Background(ui.BgLightColor)
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true).Background(ui.BgLightColor)
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle().Background(ui.BgLightColor)
	ta.BlurredStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(ui.BgLightColor)

	ta.Cursor.Style = lipgloss.NewStyle().Foreground(ui.SelectionFg).Background(ui.Primary)
	ta.Cursor.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)
}

func (f *PublishForm) SetWidth(w int) { f.width = w }

// SetHeight caps the panel's vertical size. The payload textarea shrinks to
// keep title/inputs/buttons visible; a value of 0 or less disables the cap
// (the textarea reverts to its default 5 rows).
func (f *PublishForm) SetHeight(h int) {
	if h < 0 {
		h = 0
	}
	f.height = h
}

// SetOrigin records the panel's top-left in the final view for coordinate-
// based mouse hit-testing.
func (f *PublishForm) SetOrigin(x, y int) {
	f.topX = x
	f.topY = y
}

func (f *PublishForm) Loading() bool { return f.loading }
func (f *PublishForm) Success() bool { return f.success }
func (f *PublishForm) LastSeq() uint64 { return f.lastSeq }

// AtStart reports whether ↑ should escape the form (only when focus is on
// the first field). Used by the focus-manager adapter so arrow keys can jump
// out of the form to Tabs above.
func (f *PublishForm) AtStart() bool { return f.field == pfFieldSubject }

// AtEnd reports whether ↓ should escape the form (only when focus is on the
// bottom button row: Cancel or Submit). Nothing sits below the button row
// in the current layout, but keeping the contract mirrors Form.AtEnd.
func (f *PublishForm) AtEnd() bool { return f.field == pfFieldCancel || f.field == pfFieldSubmit }

func (f *PublishForm) Reset() {
	f.subject.Reset()
	f.headers.Reset()
	f.payload.Reset()
	f.setField(pfFieldSubject)
	f.err = ""
	f.loading = false
	f.success = false
}

func (f *PublishForm) HandleResult(msg PublishResultMsg) {
	f.loading = false
	if msg.Err != nil {
		f.err = msg.Err.Error()
		return
	}
	f.success = true
	f.lastSeq = msg.Seq
}

// setField switches focus, updating the underlying bubbles models so their
// cursor blink only happens on the active input.
func (f *PublishForm) setField(idx int) {
	switch f.field {
	case pfFieldSubject:
		f.subject.Blur()
	case pfFieldHeaders:
		f.headers.Blur()
	case pfFieldPayload:
		f.payload.Blur()
	}
	f.field = idx
	switch idx {
	case pfFieldSubject:
		f.subject.Focus()
	case pfFieldHeaders:
		f.headers.Focus()
	case pfFieldPayload:
		f.payload.Focus()
	}
}

// Update returns (submitCmd, handled). Parent forwards KeyMsg here; the form
// consumes it fully.
func (f *PublishForm) Update(msg tea.Msg, client *natsclient.Client) (tea.Cmd, bool) {
	if f.loading {
		return nil, true
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}

	switch km.String() {
	case "tab":
		f.setField((f.field + 1) % pfFieldCount)
		return nil, true
	case "shift+tab":
		f.setField((f.field - 1 + pfFieldCount) % pfFieldCount)
		return nil, true
	case "ctrl+s":
		return f.submit(client), true
	case "left":
		if f.field == pfFieldSubmit {
			f.setField(pfFieldCancel)
			return nil, true
		}
	case "right":
		if f.field == pfFieldCancel {
			f.setField(pfFieldSubmit)
			return nil, true
		}
	case "up":
		// From button row → jump into payload. From single-line inputs → prev.
		// Inside payload, let it handle up if there is room to move up.
		switch f.field {
		case pfFieldHeaders:
			f.setField(pfFieldSubject)
			return nil, true
		case pfFieldPayload:
			if f.payload.Line() == 0 {
				f.setField(pfFieldHeaders)
				return nil, true
			}
		case pfFieldCancel, pfFieldSubmit:
			f.setField(pfFieldPayload)
			return nil, true
		}
	case "down":
		switch f.field {
		case pfFieldSubject:
			f.setField(pfFieldHeaders)
			return nil, true
		case pfFieldHeaders:
			f.setField(pfFieldPayload)
			return nil, true
		case pfFieldPayload:
			if f.payload.Line() >= f.payload.LineCount()-1 {
				f.setField(pfFieldSubmit)
				return nil, true
			}
		}
	}

	// Button-specific: Enter fires the button; space also cancels on Cancel.
	switch f.field {
	case pfFieldSubmit:
		if km.String() == "enter" {
			return f.submit(client), true
		}
		return nil, true
	case pfFieldCancel:
		if km.String() == "enter" || km.String() == " " || km.String() == "space" {
			return cancelCmd(), true
		}
		return nil, true
	}

	// Delegate character editing to the focused bubbles model.
	var cmd tea.Cmd
	switch f.field {
	case pfFieldSubject:
		f.subject, cmd = f.subject.Update(km)
	case pfFieldHeaders:
		f.headers, cmd = f.headers.Update(km)
	case pfFieldPayload:
		f.payload, cmd = f.payload.Update(km)
	}
	return cmd, true
}

// cancelCmd emits a synthetic Esc so the parent's existing Esc handler fires,
// keeping click/keyboard/button paths unified.
func cancelCmd() tea.Cmd {
	km := keyMsgForString("esc")
	return func() tea.Msg { return km }
}

func (f *PublishForm) submit(client *natsclient.Client) tea.Cmd {
	f.err = ""
	f.success = false
	subject := strings.TrimSpace(f.subject.Value())
	if subject == "" {
		f.err = "Subject is required"
		return nil
	}
	headers, err := parseHeaderLine(f.headers.Value())
	if err != nil {
		f.err = err.Error()
		return nil
	}
	payload := []byte(f.payload.Value())
	f.loading = true
	return func() tea.Msg {
		seq, perr := client.PublishMessage(subject, payload, headers)
		return PublishResultMsg{Seq: seq, Err: perr}
	}
}

func (f *PublishForm) View() string {
	w := f.width - 4
	if w < 40 {
		w = 40
	}
	inner := w - 4 // subtract panel Padding(1, 2) * 2 cols
	if inner < 20 {
		inner = 20
	}

	// Sync widths every render — content width can change on resize. The
	// panel's Padding(1, 2) already carved out `inner` cols; each input sits
	// in an inputBox with Padding(0, 1), so its usable text width is inner-2.
	f.subject.Width = inner - 2
	f.headers.Width = inner - 2
	f.payload.SetWidth(inner)

	// Payload textarea height: shrink to fit inside f.height when it's set,
	// otherwise keep the default 5 rows. Fixed non-textarea rows inside the
	// panel = 12 (title/hint/blank/subject-label/subject/blank/headers-label/
	// headers/blank/payload-label/blank/buttons). Panel border+padding costs
	// another 4 rows outside `lines`, so total = 12 + textarea + 4.
	payloadRows := 5
	if f.height > 0 {
		const fixedRows = 12
		const panelChrome = 4 // border(2) + padding(1,2) top/bottom
		budget := f.height - fixedRows - panelChrome
		if budget < 2 {
			budget = 2
		}
		if budget < 5 {
			payloadRows = budget
		}
	}
	f.payload.SetHeight(payloadRows)

	title := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true).
		Render("Publish to Stream: " + f.stream)
	hint := lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true).
		Render("Tab/Shift+Tab switch  Ctrl+S submit  Esc cancel")

	subjectLabel := f.fieldLabel("Subject", pfFieldSubject)
	headersLabel := f.fieldLabel("Headers", pfFieldHeaders)
	payloadLabel := f.fieldLabel("Payload", pfFieldPayload)

	// Wrap single-line inputs in a subtle bg block so the input surface is
	// visible even when empty. Textarea already has its own bg via style.
	inputBox := lipgloss.NewStyle().
		Background(ui.BgLighter).
		Padding(0, 1).
		Width(inner)

	subjectView := f.subject.View()

	// Build view line-by-line; record absolute positions used by HandleMouse.
	// panel content column-0 is at (topX + 1 border + 2 padding) = topX + 3
	contentStartX := f.topX + 3
	// panel content row-0 is at (topY + 1 border + 1 padding) = topY + 2
	contentStartY := f.topY + 2

	f.inputContentX = contentStartX + 1 // +1 for the inputBox left padding

	var lines []string
	rowsSoFar := 0

	appendLine := func(s string) {
		lines = append(lines, s)
		rowsSoFar += lipgloss.Height(s)
	}

	appendLine(title)
	appendLine(hint)
	appendLine("")
	appendLine(subjectLabel)
	f.subjectY = contentStartY + rowsSoFar
	appendLine(inputBox.Render(subjectView))
	appendLine("")
	appendLine(headersLabel)
	f.headersY = contentStartY + rowsSoFar
	appendLine(inputBox.Render(f.headers.View()))
	appendLine("")
	appendLine(payloadLabel)
	f.payloadYStart = contentStartY + rowsSoFar
	payloadView := f.payload.View()
	appendLine(payloadView)
	f.payloadYEnd = contentStartY + rowsSoFar - 1
	appendLine("")

	// Buttons row.
	cancelFocused := f.field == pfFieldCancel
	submitFocused := f.field == pfFieldSubmit
	cancelBtn := components.RenderPill("Cancel", pillState(cancelFocused))
	submitBtn := components.RenderPill("Publish", pillState(submitFocused))
	cancelW := lipgloss.Width(cancelBtn)
	submitW := lipgloss.Width(submitBtn)
	f.buttonRowY = contentStartY + rowsSoFar
	f.cancelXStart = contentStartX
	f.cancelXEnd = contentStartX + cancelW - 1
	f.submitXStart = f.cancelXEnd + 1 + 2 // "  " separator
	f.submitXEnd = f.submitXStart + submitW - 1
	actions := lipgloss.JoinHorizontal(lipgloss.Top, cancelBtn, "  ", submitBtn)
	appendLine(actions)

	if f.err != "" {
		appendLine("")
		appendLine(lipgloss.NewStyle().Foreground(ui.Error).Bold(true).
			Render("✖ " + f.err))
	}
	if f.loading {
		appendLine("")
		appendLine(lipgloss.NewStyle().Foreground(ui.SubtleColor).Italic(true).
			Render("Publishing..."))
	}
	if f.success {
		appendLine("")
		appendLine(lipgloss.NewStyle().Foreground(ui.Success).Bold(true).
			Render(fmt.Sprintf("✓ Published (seq %d). Ctrl+S again to send another, Esc to exit.", f.lastSeq)))
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.BorderColor).
		Padding(1, 2).
		Width(w).
		Render(strings.Join(lines, "\n"))

	return panel
}

func (f *PublishForm) fieldLabel(label string, field int) string {
	active := f.field == field
	style := lipgloss.NewStyle().Foreground(ui.SubtleColor).Bold(true)
	if active {
		style = style.Foreground(ui.BrandPrimary)
	}
	return style.Render(label + ":")
}

// renderFormButton has been replaced by components.RenderPill; a local
// pillState helper mirrors the components-package helper for the common
// Idle/Focused toggle used inside this file.
func pillState(focused bool) components.ButtonState {
	if focused {
		return components.ButtonFocused
	}
	return components.ButtonIdle
}

// HandleMouse routes clicks by absolute coordinates:
//   - Click on a text-field row → focus that field and move the cursor to the
//     clicked column (for single-line inputs).
//   - Click on Cancel/Publish → fire the same code path as Enter on that
//     button.
//
// Non-press events and clicks outside any of these regions return (nil, false)
// so the caller can decide whether to forward further.
func (f *PublishForm) HandleMouse(msg tea.MouseMsg, client *natsclient.Client) (tea.Cmd, bool) {
	if f.loading {
		return nil, true
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false
	}

	// Button row first — clicks on the buttons take priority over text fields.
	if inRow(msg.Y, f.buttonRowY) {
		if msg.X >= f.cancelXStart-1 && msg.X <= f.cancelXEnd+1 {
			f.setField(pfFieldCancel)
			return cancelCmd(), true
		}
		if msg.X >= f.submitXStart-1 && msg.X <= f.submitXEnd+1 {
			f.setField(pfFieldSubmit)
			return f.submit(client), true
		}
	}

	// Subject / Headers rows.
	if inRow(msg.Y, f.subjectY) {
		f.setField(pfFieldSubject)
		f.subject.SetCursor(clickCursor(msg.X, f.inputContentX, len([]rune(f.subject.Value()))))
		return nil, true
	}
	if inRow(msg.Y, f.headersY) {
		f.setField(pfFieldHeaders)
		f.headers.SetCursor(clickCursor(msg.X, f.inputContentX, len([]rune(f.headers.Value()))))
		return nil, true
	}

	// Payload textarea range.
	if msg.Y >= f.payloadYStart-1 && msg.Y <= f.payloadYEnd+1 {
		f.setField(pfFieldPayload)
		return nil, true
	}

	return nil, false
}

// inRow tolerates a ±1 Y offset (some terminals report mouse.Y one row less
// than the visible row).
func inRow(y, target int) bool {
	return y == target || y+1 == target
}

// clickCursor maps an absolute click X to a rune-offset within a single-line
// input. Clamps to [0, len].
func clickCursor(clickX, inputX, valueLen int) int {
	offset := clickX - inputX
	if offset < 0 {
		return 0
	}
	if offset > valueLen {
		return valueLen
	}
	return offset
}

// parseHeaderLine accepts "k1=v1, k2=v2" and returns map[string][]string.
// Empty input yields nil. Values are trimmed. Reports an error for entries
// missing '='.
func parseHeaderLine(s string) (map[string][]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string][]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("header %q must be k=v", part)
		}
		k := strings.TrimSpace(part[:eq])
		v := strings.TrimSpace(part[eq+1:])
		if k == "" {
			return nil, fmt.Errorf("header key is empty in %q", part)
		}
		out[k] = append(out[k], v)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
