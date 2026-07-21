package pages

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
)

type CreateKVBucketPage struct {
	client  *natsclient.Client
	form    *components.Form
	loading bool
	err     string
	success bool

	originX, originY int
}

type KVBucketCreatedMsg struct {
	Err error
}

func NewCreateKVBucketPage(client *natsclient.Client) *CreateKVBucketPage {
	fields := []components.FormField{
		{
			Label:       "Name",
			Placeholder: "e.g. session-store",
			Type:        components.FieldTypeText,
			Required:    true,
		},
		{
			Label:       "Description",
			Placeholder: "optional description",
			Type:        components.FieldTypeText,
			Required:    false,
		},
		{
			Label:       "History",
			Placeholder: "1",
			Value:       "1",
			Type:        components.FieldTypeText,
			Required:    false,
		},
		{
			Label:       "TTL",
			Placeholder: "0 or 30m, 24h, 7d",
			Value:       "0",
			Type:        components.FieldTypeText,
			Required:    false,
		},
		{
			Label:       "Replicas",
			Placeholder: "1",
			Value:       "1",
			Type:        components.FieldTypeText,
			Required:    false,
		},
	}

	form := components.NewForm("Create KV Bucket", fields)
	form.Focus()

	return &CreateKVBucketPage{client: client, form: form}
}

func (p *CreateKVBucketPage) Init() tea.Cmd { return nil }

func (p *CreateKVBucketPage) Update(msg tea.Msg) (*CreateKVBucketPage, tea.Cmd) {
	switch msg := msg.(type) {
	case KVBucketCreatedMsg:
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

func (p *CreateKVBucketPage) handleSubmit() (*CreateKVBucketPage, tea.Cmd) {
	if !p.form.Validate() {
		return p, nil
	}
	v := p.form.Values()

	name := v["Name"]
	description := v["Description"]

	// History: JetStream KV supports 1..64 (uint8 config field capped by
	// nats-server). Validate here so the user gets a clear error before
	// the round-trip.
	history := int64(1)
	if h := trimSpace(v["History"]); h != "" {
		n, err := strconv.ParseInt(h, 10, 64)
		if err != nil || n < 1 || n > 64 {
			p.form.SetError("History must be an integer between 1 and 64")
			return p, nil
		}
		history = n
	}

	// TTL: accept Go duration syntax, or "0" (or empty) for no expiry.
	var ttl time.Duration
	if t := trimSpace(v["TTL"]); t != "" && t != "0" {
		d, err := time.ParseDuration(t)
		if err != nil {
			p.form.SetError("TTL must be a duration like 30m, 24h, or 0 for none")
			return p, nil
		}
		ttl = d
	}

	replicas := 1
	if r := trimSpace(v["Replicas"]); r != "" {
		n, err := strconv.Atoi(r)
		if err != nil || n < 1 || n > 5 {
			p.form.SetError("Replicas must be an integer between 1 and 5")
			return p, nil
		}
		replicas = n
	}

	p.loading = true
	return p, func() tea.Msg {
		err := p.client.CreateKVBucket(name, description, history, ttl, replicas)
		return KVBucketCreatedMsg{Err: err}
	}
}

func (p *CreateKVBucketPage) SetSize(width, height int) {
	p.form.SetWidth(width)
	p.form.SetHeight(height)
}

func (p *CreateKVBucketPage) SetContentOrigin(x, y int) {
	p.originX = x
	p.originY = y
}

func (p *CreateKVBucketPage) View() string {
	if p.success {
		return "\n  KV bucket created successfully!\n\n  Press Esc to go back"
	}
	if p.loading {
		return "\n  Creating KV bucket..."
	}
	p.form.SetOrigin(p.originX, p.originY)
	return p.form.View()
}

func (p *CreateKVBucketPage) HelpText() string {
	return "Tab: Next  Shift+Tab: Prev  Enter: Submit  Esc: Cancel"
}

func (p *CreateKVBucketPage) StatusText() string {
	if p.loading {
		return "Creating..."
	}
	return "Create KV Bucket"
}

func (p *CreateKVBucketPage) IsSuccess() bool { return p.success }

func (p *CreateKVBucketPage) FocusItems() []focus.Item {
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

func (p *CreateKVBucketPage) FocusRouted() bool { return !p.success }
