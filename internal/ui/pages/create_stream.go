package pages

import (
	tea "github.com/charmbracelet/bubbletea"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
)

type CreateStreamPage struct {
	client  *natsclient.Client
	form    *components.Form
	loading bool
	err     string
	success bool

	originX, originY int
}

type StreamCreatedMsg struct {
	Err error
}

func NewCreateStreamPage(client *natsclient.Client) *CreateStreamPage {
	fields := []components.FormField{
		{
			Label:       "Name",
			Placeholder: "e.g. ORDERS",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    true,
		},
		{
			Label:       "Subjects",
			Placeholder: "e.g. orders.* (comma separated)",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    true,
		},
		{
			Label:       "Description",
			Placeholder: "optional description",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    false,
		},
		{
			Label:       "Storage",
			Type:        components.FieldTypeSelect,
			Options:     []string{"file", "memory"},
			SelectedOpt: 0,
			Required:    true,
		},
		{
			Label:       "Replicas",
			Placeholder: "1",
			Value:       "1",
			Type:        components.FieldTypeText,
			Required:    false,
		},
	}

	form := components.NewForm("Create Stream", fields)
	form.Focus()

	return &CreateStreamPage{
		client: client,
		form:   form,
	}
}

func (p *CreateStreamPage) Init() tea.Cmd {
	return nil
}

func (p *CreateStreamPage) Update(msg tea.Msg) (*CreateStreamPage, tea.Cmd) {
	switch msg := msg.(type) {
	case StreamCreatedMsg:
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

		switch msg.String() {
		case "enter":
			if p.form.IsCancelFocused() {
				return p, cancelCmd()
			}
			return p.handleSubmit()
		}
	}

	_, cmd := p.form.Update(msg)
	return p, cmd
}

func (p *CreateStreamPage) handleSubmit() (*CreateStreamPage, tea.Cmd) {
	if !p.form.Validate() {
		return p, nil
	}

	values := p.form.Values()

	name := values["Name"]
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
	return p, func() tea.Msg {
		err := p.client.CreateStream(name, subjects, description)
		return StreamCreatedMsg{Err: err}
	}
}

func (p *CreateStreamPage) SetSize(width, height int) {
	p.form.SetWidth(width)
	p.form.SetHeight(height)
}

// SetContentOrigin forwards the page-content top-left to the form so it can
// hit-test mouse clicks by absolute coordinates.
func (p *CreateStreamPage) SetContentOrigin(x, y int) {
	p.originX = x
	p.originY = y
}

func (p *CreateStreamPage) View() string {
	if p.success {
		return "\n  Stream created successfully!\n\n  Press Esc to go back"
	}
	if p.loading {
		return "\n  Creating stream..."
	}
	p.form.SetOrigin(p.originX, p.originY)
	return p.form.View()
}

func (p *CreateStreamPage) HelpText() string {
	return "Tab: Next  Shift+Tab: Prev  Enter: Submit  Esc: Cancel"
}

func (p *CreateStreamPage) StatusText() string {
	if p.loading {
		return "Creating..."
	}
	return "Create Stream"
}

func (p *CreateStreamPage) IsSuccess() bool {
	return p.success
}

// FocusItems returns the form as a single opaque focus item. Enter on
// Cancel emits the same synthetic cancel as the legacy handler; Enter on
// Submit runs handleSubmit.
func (p *CreateStreamPage) FocusItems() []focus.Item {
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
func (p *CreateStreamPage) FocusRouted() bool { return !p.success }

func splitSubjects(s string) []string {
	var result []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
