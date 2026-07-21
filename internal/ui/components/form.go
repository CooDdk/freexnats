package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/ui"
)

type FieldType int

const (
	FieldTypeText FieldType = iota
	FieldTypeSelect
)

// FormField declares one row in a form. For text fields, Value is the initial
// value; runtime edits live inside an internal textinput.Model and are read
// back via Form.Values() / Form.FieldValue().
type FormField struct {
	Label       string
	Placeholder string
	Value       string
	Type        FieldType
	Options     []string
	SelectedOpt int
	Required    bool
	Help        string
}

// Form is a keyboard + mouse driven form. Text fields delegate editing to
// bubbles/textinput (real cursor, Unicode, copy/paste). Layout positions are
// recorded during View() so mouse clicks can hit-test fields, select options,
// and Cancel/Submit buttons by absolute coordinates.
type Form struct {
	fields      []FormField
	inputs      []textinput.Model // parallel to fields; zero value for Select rows
	activeField int
	focused     bool
	title       string
	width       int
	height      int // vertical budget for View(); 0 = unbounded (legacy behavior)
	scrollOff   int // top-of-window line index when the form is taller than height
	err         string

	// Panel top-left in the final view. Set by SetOrigin every render.
	topX int
	topY int

	// Recorded during View() for coordinate-based mouse hit-testing.
	fieldY         []int   // absolute Y of each field row
	inputContentX  int     // absolute X where text field content starts
	selectRangesAt map[int][][2]int
	buttonRowY     int
	cancelXStart   int
	cancelXEnd     int
	submitXStart   int
	submitXEnd     int
}

// formLeftPad is the number of columns every form row is indented by.
// Labels, inputs, help lines, and the button row all sit at (topX+formLeftPad).
const formLeftPad = 2

func NewForm(title string, fields []FormField) *Form {
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		if f.Type != FieldTypeText {
			continue
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = f.Placeholder
		ti.SetValue(f.Value)
		ti.CursorEnd()
		applyFormInputStyles(&ti)
		inputs[i] = ti
	}
	f := &Form{
		title:          title,
		fields:         fields,
		inputs:         inputs,
		selectRangesAt: map[int][][2]int{},
	}
	f.syncFocus()
	return f
}

func applyFormInputStyles(ti *textinput.Model) {
	ti.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(ui.SelectionFg).Background(ui.Primary)
	ti.Cursor.TextStyle = lipgloss.NewStyle().Foreground(ui.TextColor)
}

func (f *Form) SetWidth(width int) {
	f.width = width
}

// SetHeight sets the vertical budget for View(). When the form's rendered
// height exceeds this budget, View() scrolls internally to keep the active
// field / button visible. A budget of ≤0 disables scrolling (renders every
// row, unbounded).
func (f *Form) SetHeight(height int) {
	if height < 0 {
		height = 0
	}
	f.height = height
}

// SetOrigin records the form's top-left absolute position in the final view.
// Callers should set this every render before View().
func (f *Form) SetOrigin(x, y int) {
	f.topX = x
	f.topY = y
}

func (f *Form) Focus() {
	f.focused = true
	if f.activeField >= len(f.fields)+2 {
		f.activeField = 0
	}
	f.syncFocus()
}

func (f *Form) Blur() {
	f.focused = false
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
}

func (f *Form) Focused() bool { return f.focused }

// syncFocus makes only the active textinput Focused so cursor blink is
// scoped to the row the user is on.
func (f *Form) syncFocus() {
	for i := range f.inputs {
		if f.focused && i == f.activeField && f.fields[i].Type == FieldTypeText {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
}

// setActive changes activeField and refreshes textinput focus. Idempotent.
func (f *Form) setActive(idx int) {
	if idx < 0 || idx >= len(f.fields)+2 {
		return
	}
	f.activeField = idx
	f.syncFocus()
}

// SetFieldValue updates both the FormField metadata and (for Text fields) the
// underlying textinput. Use this for programmatic value changes; keyboard
// edits already flow through Update.
func (f *Form) SetFieldValue(idx int, value string) {
	if idx < 0 || idx >= len(f.fields) {
		return
	}
	f.fields[idx].Value = value
	if f.fields[idx].Type == FieldTypeText {
		f.inputs[idx].SetValue(value)
		f.inputs[idx].CursorEnd()
	}
}

// Values returns a Label→string map. Text values come from the live
// textinput; select values from Options[SelectedOpt].
func (f *Form) Values() map[string]string {
	values := make(map[string]string, len(f.fields))
	for i, field := range f.fields {
		if field.Type == FieldTypeSelect {
			if field.SelectedOpt >= 0 && field.SelectedOpt < len(field.Options) {
				values[field.Label] = field.Options[field.SelectedOpt]
			}
		} else {
			values[field.Label] = f.inputs[i].Value()
		}
	}
	return values
}

func (f *Form) Validate() bool {
	f.err = ""
	for i, field := range f.fields {
		if !field.Required {
			continue
		}
		if field.Type == FieldTypeSelect {
			if field.SelectedOpt < 0 || field.SelectedOpt >= len(field.Options) {
				f.err = field.Label + " is required"
				return false
			}
			continue
		}
		if strings.TrimSpace(f.inputs[i].Value()) == "" {
			f.err = field.Label + " is required"
			return false
		}
	}
	return true
}

func (f *Form) Error() string        { return f.err }
func (f *Form) SetError(err string)  { f.err = err }
func (f *Form) ActiveField() int     { return f.activeField }
func (f *Form) FieldCount() int      { return len(f.fields) }
func (f *Form) Fields() []FormField  { return f.fields }
func (f *Form) IsCancelFocused() bool { return f.activeField == len(f.fields) }
func (f *Form) IsSubmitFocused() bool { return f.activeField == len(f.fields)+1 }

// AtStart reports whether the active row is the very first field. Used by
// focus.Manager adapters to decide when ↑ should escape the form.
func (f *Form) AtStart() bool { return f.activeField == 0 }

// AtEnd reports whether the active row is the Submit button (past Cancel).
// Used by focus.Manager adapters to decide when ↓ should escape the form.
func (f *Form) AtEnd() bool { return f.activeField == len(f.fields)+1 }

// Arrow simulates an arrow-key press so the whole form can be treated as
// one opaque focus.Item. Delegates to Update with a synthesized tea.KeyMsg
// so all the existing arrow behavior (field navigation, Select toggling,
// Cancel↔Submit, text cursor movement) stays inside Form.
func (f *Form) Arrow(dir string) tea.Cmd {
	var t tea.KeyType
	switch dir {
	case "up":
		t = tea.KeyUp
	case "down":
		t = tea.KeyDown
	case "left":
		t = tea.KeyLeft
	case "right":
		t = tea.KeyRight
	default:
		return nil
	}
	_, cmd := f.Update(tea.KeyMsg{Type: t})
	return cmd
}

// Update handles keyboard input. Text-field editing is delegated to bubbles/
// textinput. Cancel/Submit rows have no input, only navigation.
func (f *Form) Update(msg tea.Msg) (*Form, tea.Cmd) {
	if !f.focused || len(f.fields) == 0 {
		return f, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	cancelIdx := len(f.fields)
	submitIdx := cancelIdx + 1
	total := submitIdx + 1

	switch km.String() {
	case "tab":
		f.setActive((f.activeField + 1) % total)
		return f, nil
	case "shift+tab":
		f.setActive((f.activeField - 1 + total) % total)
		return f, nil
	case "up":
		if f.activeField > 0 {
			f.setActive(f.activeField - 1)
		}
		return f, nil
	case "down":
		if f.activeField < total-1 {
			f.setActive(f.activeField + 1)
		}
		return f, nil
	case "left":
		if f.activeField == submitIdx {
			f.setActive(cancelIdx)
			return f, nil
		}
		if f.activeField < len(f.fields) && f.fields[f.activeField].Type == FieldTypeSelect {
			if f.fields[f.activeField].SelectedOpt > 0 {
				f.fields[f.activeField].SelectedOpt--
			}
			return f, nil
		}
	case "right":
		if f.activeField == cancelIdx {
			f.setActive(submitIdx)
			return f, nil
		}
		if f.activeField < len(f.fields) && f.fields[f.activeField].Type == FieldTypeSelect {
			if f.fields[f.activeField].SelectedOpt < len(f.fields[f.activeField].Options)-1 {
				f.fields[f.activeField].SelectedOpt++
			}
			return f, nil
		}
	}

	// Text field: delegate typing/backspace/cursor movement to bubbles.
	if f.activeField < len(f.fields) && f.fields[f.activeField].Type == FieldTypeText {
		var cmd tea.Cmd
		f.inputs[f.activeField], cmd = f.inputs[f.activeField].Update(km)
		return f, cmd
	}
	return f, nil
}

// HandleMouse routes clicks to fields, select options, or Cancel/Submit.
// Returns (cmd, handled). A Cancel click emits a synthetic Esc; a Submit
// click emits a synthetic Enter — reusing the page's existing keyboard
// paths so click and keyboard stay unified.
func (f *Form) HandleMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false
	}
	if !f.focused {
		return nil, false
	}

	// Button row first — priority over any field row that might share Y ±1.
	if inRow(msg.Y, f.buttonRowY) {
		if msg.X >= f.cancelXStart-1 && msg.X <= f.cancelXEnd+1 {
			f.setActive(len(f.fields))
			return synthKey("esc"), true
		}
		if msg.X >= f.submitXStart-1 && msg.X <= f.submitXEnd+1 {
			f.setActive(len(f.fields) + 1)
			return synthKey("enter"), true
		}
	}

	// Field rows.
	for i, y := range f.fieldY {
		if !inRow(msg.Y, y) {
			continue
		}
		f.setActive(i)
		if f.fields[i].Type == FieldTypeText {
			pos := msg.X - f.inputContentX
			valLen := len([]rune(f.inputs[i].Value()))
			if pos < 0 {
				pos = 0
			}
			if pos > valLen {
				pos = valLen
			}
			f.inputs[i].SetCursor(pos)
			return nil, true
		}
		// Select: pick the option under the cursor if any.
		ranges := f.selectRangesAt[i]
		for k, r := range ranges {
			if msg.X >= r[0] && msg.X <= r[1] {
				f.fields[i].SelectedOpt = k
				return nil, true
			}
		}
		return nil, true
	}

	return nil, false
}

// synthKey emits a synthetic key event so mouse clicks can reuse the page's
// keyboard handlers.
func synthKey(name string) tea.Cmd {
	var km tea.KeyMsg
	switch name {
	case "esc":
		km = tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		km = tea.KeyMsg{Type: tea.KeyEnter}
	default:
		km = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
	return func() tea.Msg { return km }
}

func (f *Form) View() string {
	// Layout: match the Publish form's spacious style — label on its own row
	// above the input, blank line between fields. Content is indented by
	// formLeftPad cols so labels and inputs sit in a comfortable column.

	inputWidth := f.width - formLeftPad*2
	if inputWidth < 24 {
		inputWidth = 24
	}
	for i := range f.inputs {
		if f.fields[i].Type == FieldTypeText {
			f.inputs[i].Width = inputWidth - 2 // account for inputBox Padding(0,1)
		}
	}

	f.fieldY = make([]int, len(f.fields))
	f.selectRangesAt = map[int][][2]int{}

	// Absolute column where a field's input/select row begins.
	contentStartX := f.topX + formLeftPad
	// Text inputs sit inside inputBox with Padding(0,1) → text content starts
	// one column to the right of the box's left edge.
	f.inputContentX = contentStartX + 1

	// localFieldLine[i] = line index (in the fully-built lines slice) of
	// field i's input row. localButtonLine = the Cancel/Submit row.
	localFieldLine := make([]int, len(f.fields))
	var localButtonLine int

	var lines []string
	appendLine := func(s string) {
		lines = append(lines, s)
	}
	leftPad := strings.Repeat(" ", formLeftPad)

	titleStyle := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true).Render(f.title)
	appendLine(leftPad + titleStyle)

	hintTop := lipgloss.NewStyle().
		Foreground(ui.TextFaint).
		Italic(true).
		Render("Tab/Shift+Tab switch  ←/→ pick option  Enter submit  Esc cancel")
	appendLine(leftPad + hintTop)
	appendLine("")

	for i, field := range f.fields {
		isActive := i == f.activeField && f.focused

		// Label line — bold and highlighted when the field is active, so
		// keyboard focus is visually obvious.
		var labelStyle lipgloss.Style
		if isActive {
			labelStyle = lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true)
		} else {
			labelStyle = lipgloss.NewStyle().Foreground(ui.SubtleColor).Bold(true)
		}
		labelText := field.Label
		if field.Required {
			labelText += " *"
		}
		marker := "  "
		if isActive {
			marker = lipgloss.NewStyle().Foreground(ui.BrandPrimary).Render("▶ ")
		}
		appendLine(leftPad + marker + labelStyle.Render(labelText))

		// Input/select row — remember its line index for scroll math and for
		// mouse hit-testing below.
		localFieldLine[i] = len(lines)
		var input string
		if field.Type == FieldTypeSelect {
			input = f.renderSelectField(i, field, isActive, contentStartX)
		} else {
			input = f.renderTextInput(i, isActive, inputWidth)
		}
		appendLine(leftPad + input)

		if field.Help != "" {
			helpText := lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true).Render(field.Help)
			appendLine(leftPad + helpText)
		}
		// Blank separator between fields — the "宽松、大气" spacing the user asked for.
		appendLine("")
	}

	cancelActive := f.IsCancelFocused() && f.focused
	submitActive := f.IsSubmitFocused() && f.focused
	cancelBtn := RenderPill("Cancel", pillState(cancelActive))
	submitBtn := RenderPill("Submit", pillState(submitActive))
	cancelW := lipgloss.Width(cancelBtn)
	submitW := lipgloss.Width(submitBtn)

	localButtonLine = len(lines)
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, cancelBtn, "  ", submitBtn)
	appendLine(leftPad + buttons)

	if f.err != "" {
		appendLine("")
		appendLine(leftPad + lipgloss.NewStyle().
			Foreground(ui.Error).
			Bold(true).
			Render("✖ "+f.err))
	}

	// Scroll: when the form is taller than the height budget, keep the active
	// field / button row inside the visible window by adjusting f.scrollOff.
	// A budget of 0 disables scrolling entirely (legacy behavior).
	if f.height > 0 && len(lines) > f.height {
		activeLine := localButtonLine
		if f.activeField < len(f.fields) {
			activeLine = localFieldLine[f.activeField]
		}
		// Anchor the window: if active row is above the window, snap top to it;
		// if below, snap bottom to it. Two-line margin at each edge lets the
		// user see the neighbouring field for context (label above / blank below).
		const margin = 2
		if activeLine-margin < f.scrollOff {
			f.scrollOff = activeLine - margin
		}
		if activeLine+margin >= f.scrollOff+f.height {
			f.scrollOff = activeLine + margin - f.height + 1
		}
		if f.scrollOff < 0 {
			f.scrollOff = 0
		}
		maxOff := len(lines) - f.height
		if f.scrollOff > maxOff {
			f.scrollOff = maxOff
		}
		lines = lines[f.scrollOff : f.scrollOff+f.height]
	} else {
		f.scrollOff = 0
	}

	// Record absolute Y for hit-testing, using post-scroll positions. Lines
	// scrolled off-screen get Y values outside the visible range, so mouse
	// clicks naturally can't hit them.
	for i := range f.fieldY {
		f.fieldY[i] = f.topY + localFieldLine[i] - f.scrollOff
	}
	f.buttonRowY = f.topY + localButtonLine - f.scrollOff
	f.cancelXStart = contentStartX
	f.cancelXEnd = f.cancelXStart + cancelW - 1
	f.submitXStart = f.cancelXEnd + 1 + 2 // "  " separator
	f.submitXEnd = f.submitXStart + submitW - 1

	return strings.Join(lines, "\n")
}

// renderTextInput wraps the bubbles textinput in a subtle bg block so an
// empty field is still visibly a "surface". Matches the Publish form style.
func (f *Form) renderTextInput(idx int, active bool, width int) string {
	bg := ui.BgLightColor
	if active {
		bg = ui.BgLighter
	}
	inputBox := lipgloss.NewStyle().
		Background(bg).
		Padding(0, 1).
		Width(width)
	return inputBox.Render(f.inputs[idx].View())
}

func (f *Form) renderSelectField(idx int, field FormField, active bool, startX int) string {
	var parts []string
	ranges := make([][2]int, 0, len(field.Options))
	cursorX := startX
	for i, opt := range field.Options {
		var rendered string
		optText := " " + opt + " "
		if i == field.SelectedOpt {
			if active {
				rendered = lipgloss.NewStyle().
					Foreground(ui.SelectionFg).
					Background(ui.Primary).
					Bold(true).
					Padding(0, 2).
					Render(optText)
			} else {
				rendered = lipgloss.NewStyle().
					Foreground(ui.PrimaryLight).
					Bold(true).
					Padding(0, 2).
					Render(optText)
			}
		} else {
			rendered = lipgloss.NewStyle().
				Foreground(ui.TextMuted).
				Background(ui.BgLightColor).
				Padding(0, 2).
				Render(optText)
		}
		w := lipgloss.Width(rendered)
		ranges = append(ranges, [2]int{cursorX, cursorX + w - 1})
		cursorX += w
		// One-column gap between options so unselected pills read as separate
		// clickable regions rather than a single strip. The gap itself is not
		// hit-testable — clicks in the gap fall through.
		if i < len(field.Options)-1 {
			parts = append(parts, rendered, " ")
			cursorX++
		} else {
			parts = append(parts, rendered)
		}
	}
	f.selectRangesAt[idx] = ranges
	return strings.Join(parts, "")
}

// inRow tolerates a ±1 Y offset from the recorded row in either direction —
// some terminals report a mouse Y that's one row off from the visible row, and
// the extra tolerance is safe here because we render a blank line between
// every field so adjacent rows are always ≥2 apart.
func inRow(y, target int) bool {
	return y >= target-1 && y <= target+1
}
