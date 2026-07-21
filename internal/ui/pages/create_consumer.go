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

type CreateConsumerPage struct {
	client     *natsclient.Client
	streamName string
	form       *components.Form
	loading    bool
	err        string
	success    bool

	originX, originY int
}

type ConsumerCreatedMsg struct {
	Err error
}

func NewCreateConsumerPage(client *natsclient.Client, streamName string) *CreateConsumerPage {
	fields := []components.FormField{
		{
			Label:       "Name",
			Placeholder: "e.g. orders-processor",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    true,
		},
		{
			Label:       "Filter Subject",
			Placeholder: "e.g. orders.> (empty = all)",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    false,
		},
		{
			Label:       "Description",
			Placeholder: "optional description",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    false,
		},
		{
			Label:       "Ack Policy",
			Type:        components.FieldTypeSelect,
			Options:     []string{"explicit", "all", "none"},
			SelectedOpt: 0,
			Required:    true,
		},
		{
			Label:       "Deliver Policy",
			Type:        components.FieldTypeSelect,
			Options:     []string{"all", "last", "new"},
			SelectedOpt: 0,
			Required:    true,
		},
	}

	form := components.NewForm("Create Consumer", fields)
	form.Focus()

	return &CreateConsumerPage{
		client:     client,
		streamName: streamName,
		form:       form,
	}
}

func (p *CreateConsumerPage) Init() tea.Cmd {
	return nil
}

func (p *CreateConsumerPage) Update(msg tea.Msg) (*CreateConsumerPage, tea.Cmd) {
	switch msg := msg.(type) {
	case ConsumerCreatedMsg:
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

func (p *CreateConsumerPage) handleSubmit() (*CreateConsumerPage, tea.Cmd) {
	if !p.form.Validate() {
		return p, nil
	}

	values := p.form.Values()

	name := values["Name"]
	filterSubject := values["Filter Subject"]
	description := values["Description"]
	ackPolicy := values["Ack Policy"]
	deliverPolicy := values["Deliver Policy"]

	// Name doubles as durable name — JetStream requires them to match anyway.
	durableName := name

	p.loading = true
	return p, func() tea.Msg {
		err := p.client.CreateConsumer(p.streamName, name, filterSubject, durableName, description, ackPolicy, deliverPolicy)
		return ConsumerCreatedMsg{Err: err}
	}
}

func (p *CreateConsumerPage) SetSize(width, height int) {
	p.form.SetWidth(width)
	p.form.SetHeight(height)
}

// SetContentOrigin records the page-content top-left; the form's absolute
// origin is derived in View() by adding the pre-form header row count.
func (p *CreateConsumerPage) SetContentOrigin(x, y int) {
	p.originX = x
	p.originY = y
}

func (p *CreateConsumerPage) View() string {
	if p.success {
		return "\n  Consumer created successfully!\n\n  Press Esc to go back"
	}
	if p.loading {
		return "\n  Creating consumer..."
	}

	labelStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(ui.TextColor)
	header := "  " + labelStyle.Render("Stream:") + " " + valueStyle.Render(p.streamName) + "\n\n"

	p.form.SetOrigin(p.originX, p.originY+strings.Count(header, "\n"))
	return header + p.form.View()
}

func (p *CreateConsumerPage) HelpText() string {
	return "Tab: Next  Shift+Tab: Prev  Enter: Submit  Esc: Cancel"
}

func (p *CreateConsumerPage) StatusText() string {
	if p.loading {
		return "Creating..."
	}
	return "Create Consumer (stream: " + p.streamName + ")"
}

func (p *CreateConsumerPage) IsSuccess() bool {
	return p.success
}

// FocusItems returns the form as a single opaque focus item.
func (p *CreateConsumerPage) FocusItems() []focus.Item {
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
func (p *CreateConsumerPage) FocusRouted() bool { return !p.success }
