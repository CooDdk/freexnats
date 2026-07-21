package pages

import (
	tea "github.com/charmbracelet/bubbletea"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
)

// KVKeyFormPage handles both creating a new key and editing an existing key
// in a KV bucket. In edit mode the Key field is locked (edits go to the
// Value only); in create mode both fields are user-supplied.
type KVKeyFormPage struct {
	client  *natsclient.Client
	bucket  string
	editing bool
	origKey string

	form    *components.Form
	loading bool
	err     string
	success bool

	originX, originY int
}

type KVKeyPutMsg struct {
	Key string
	Err error
}

func NewCreateKVKeyPage(client *natsclient.Client, bucket string) *KVKeyFormPage {
	fields := []components.FormField{
		{
			Label:       "Key",
			Placeholder: "e.g. user.42.profile",
			Type:        components.FieldTypeText,
			Required:    true,
		},
		{
			Label:       "Value",
			Placeholder: "raw string or JSON payload",
			Type:        components.FieldTypeText,
			Required:    false,
		},
	}

	form := components.NewForm("Create KV Key — "+bucket, fields)
	form.Focus()

	return &KVKeyFormPage{
		client:  client,
		bucket:  bucket,
		editing: false,
		form:    form,
	}
}

func NewEditKVKeyPage(client *natsclient.Client, bucket, key, value string) *KVKeyFormPage {
	fields := []components.FormField{
		{
			Label:       "Value",
			Placeholder: "raw string or JSON payload",
			Value:       value,
			Type:        components.FieldTypeText,
			Required:    false,
		},
	}

	form := components.NewForm("Edit KV Key — "+bucket+"/"+key, fields)
	form.Focus()

	return &KVKeyFormPage{
		client:  client,
		bucket:  bucket,
		editing: true,
		origKey: key,
		form:    form,
	}
}

func (p *KVKeyFormPage) Init() tea.Cmd { return nil }

func (p *KVKeyFormPage) Update(msg tea.Msg) (*KVKeyFormPage, tea.Cmd) {
	switch msg := msg.(type) {
	case KVKeyPutMsg:
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

func (p *KVKeyFormPage) handleSubmit() (*KVKeyFormPage, tea.Cmd) {
	if !p.form.Validate() {
		return p, nil
	}
	v := p.form.Values()

	var key, value string
	if p.editing {
		key = p.origKey
		value = v["Value"]
	} else {
		key = trimSpace(v["Key"])
		if key == "" {
			p.form.SetError("Key is required")
			return p, nil
		}
		value = v["Value"]
	}

	p.loading = true
	return p, func() tea.Msg {
		_, err := p.client.PutKVValue(p.bucket, key, value)
		return KVKeyPutMsg{Key: key, Err: err}
	}
}

func (p *KVKeyFormPage) SetSize(width, height int) {
	p.form.SetWidth(width)
	p.form.SetHeight(height)
}

func (p *KVKeyFormPage) SetContentOrigin(x, y int) {
	p.originX = x
	p.originY = y
}

func (p *KVKeyFormPage) View() string {
	if p.success {
		if p.editing {
			return "\n  KV key updated successfully!\n\n  Press Esc to go back"
		}
		return "\n  KV key created successfully!\n\n  Press Esc to go back"
	}
	if p.loading {
		if p.editing {
			return "\n  Updating KV key..."
		}
		return "\n  Creating KV key..."
	}
	p.form.SetOrigin(p.originX, p.originY)
	return p.form.View()
}

func (p *KVKeyFormPage) HelpText() string {
	return "Tab: Next  Shift+Tab: Prev  Enter: Submit  Esc: Cancel"
}

func (p *KVKeyFormPage) StatusText() string {
	if p.loading {
		if p.editing {
			return "Updating..."
		}
		return "Creating..."
	}
	if p.editing {
		return "Edit KV Key — " + p.bucket + "/" + p.origKey
	}
	return "Create KV Key — " + p.bucket
}

func (p *KVKeyFormPage) IsSuccess() bool { return p.success }

// IsEditing reports whether this form is an edit (vs create) instance —
// used by app.go to distinguish the refresh target after a successful put.
func (p *KVKeyFormPage) IsEditing() bool { return p.editing }

// Bucket returns the bucket this form targets, so app.go can refresh the
// correct key list after a successful put.
func (p *KVKeyFormPage) Bucket() string { return p.bucket }

func (p *KVKeyFormPage) FocusItems() []focus.Item {
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

func (p *KVKeyFormPage) FocusRouted() bool { return !p.success }
