package pages

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
)

type EditStreamPage struct {
	client   *natsclient.Client
	original *natsclient.StreamInfo
	form     *components.Form
	loading  bool
	err      string
	success  bool

	originX, originY int
}

type StreamUpdatedMsg struct {
	Err error
}

func NewEditStreamPage(client *natsclient.Client, info *natsclient.StreamInfo) *EditStreamPage {
	fields := []components.FormField{
		{
			Label:       "Subjects",
			Placeholder: "comma separated, e.g. orders.*, cart.>",
			Value:       joinSubjectsCSV(info.Subjects),
			Type:        components.FieldTypeText,
			Required:    true,
		},
		{
			Label:       "Description",
			Placeholder: "optional description",
			Value:       info.Description,
			Type:        components.FieldTypeText,
			Required:    false,
		},
	}

	form := components.NewForm("Edit Stream", fields)
	form.Focus()

	return &EditStreamPage{
		client:   client,
		original: info,
		form:     form,
	}
}

func (p *EditStreamPage) Init() tea.Cmd { return nil }

func (p *EditStreamPage) Update(msg tea.Msg) (*EditStreamPage, tea.Cmd) {
	switch msg := msg.(type) {
	case StreamUpdatedMsg:
		p.loading = false
		if msg.Err != nil {
			p.err = msg.Err.Error()
			p.form.SetError(p.err)
			return p, nil
		}
		p.success = true
		return p, nil

	case tea.MouseMsg:
		if cmd, handled := p.form.HandleMouse(msg); handled {
			return p, cmd
		}
		return p, nil

	case tea.KeyMsg:
		if p.success {
			return p, nil
		}
		if msg.String() == "enter" {
			if p.form.IsCancelFocused() {
				return p, cancelCmd()
			}
			return p.handleSubmit()
		}
	}

	_, cmd := p.form.Update(msg)
	return p, cmd
}

func (p *EditStreamPage) handleSubmit() (*EditStreamPage, tea.Cmd) {
	if !p.form.Validate() {
		return p, nil
	}

	values := p.form.Values()
	subjectsStr := values["Subjects"]
	description := values["Description"]

	var subjects []string
	for _, s := range splitSubjects(subjectsStr) {
		s = trimSpace(s)
		if s != "" {
			subjects = append(subjects, s)
		}
	}
	if len(subjects) == 0 {
		p.form.SetError("At least one subject is required")
		return p, nil
	}

	p.loading = true
	name := p.original.Name
	return p, func() tea.Msg {
		err := p.client.UpdateStream(name, subjects, description)
		return StreamUpdatedMsg{Err: err}
	}
}

func (p *EditStreamPage) SetSize(width, height int) {
	p.form.SetWidth(width)
	p.form.SetHeight(height)
}

// SetContentOrigin records the page-content top-left in the final view.
// The form's absolute origin is derived in View() by adding the pre-form
// header row count.
func (p *EditStreamPage) SetContentOrigin(x, y int) {
	p.originX = x
	p.originY = y
}

func (p *EditStreamPage) View() string {
	if p.success {
		return "\n  Stream updated successfully!\n\n  Press Esc to go back"
	}
	if p.loading {
		return "\n  Updating stream..."
	}

	labelStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(ui.TextColor)
	dim := lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true)

	header := strings.Join([]string{
		"  " + labelStyle.Render("Stream:") + " " + valueStyle.Render(p.original.Name) +
			"    " + labelStyle.Render("Storage:") + " " + valueStyle.Render(p.original.Storage),
		"  " + dim.Render("Name and Storage cannot be changed here."),
		"",
	}, "\n")

	// Form's Y sits below the header. strings.Count of newlines equals the
	// number of visible rows preceding the form's first line.
	p.form.SetOrigin(p.originX, p.originY+strings.Count(header, "\n"))
	return header + p.form.View()
}

func (p *EditStreamPage) HelpText() string {
	return "Tab: Next  Shift+Tab: Prev  Enter: Submit  Esc: Cancel"
}

func (p *EditStreamPage) StatusText() string {
	if p.loading {
		return "Updating..."
	}
	return "Edit Stream (" + p.original.Name + ")"
}

func (p *EditStreamPage) IsSuccess() bool { return p.success }

// FocusItems returns the form as one opaque focus item, mirroring
// CreateStreamPage / SettingsPage.
func (p *EditStreamPage) FocusItems() []focus.Item {
	return []focus.Item{
		NewFormFocusItem(p.form, func() tea.Cmd {
			if p.form.IsCancelFocused() {
				return cancelCmd()
			}
			_, cmd := p.handleSubmit()
			return cmd
		}),
	}
}

// FocusRouted disables routing when the success screen is up.
func (p *EditStreamPage) FocusRouted() bool { return !p.success }

func joinSubjectsCSV(subjects []string) string {
	out := ""
	for i, s := range subjects {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
