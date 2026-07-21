package pages

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
)

type SettingsPage struct {
	client     *natsclient.Client
	form       *components.Form
	connected  bool
	connecting bool
	serverInfo string

	originX, originY int
}

func NewSettingsPage(client *natsclient.Client) *SettingsPage {
	fields := []components.FormField{
		{
			Label:       "Server URL",
			Placeholder: "e.g. nats://localhost:4222",
			Value:       "nats://localhost:4222",
			Type:        components.FieldTypeText,
			Required:    true,
			Help:        "The NATS server address to connect to",
		},
		{
			Label:       "Username",
			Placeholder: "optional username",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    false,
			Help:        "Username for authentication (optional)",
		},
		{
			Label:       "Password",
			Placeholder: "optional password",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    false,
			Help:        "Password for authentication (optional)",
		},
		{
			Label:       "Token",
			Placeholder: "optional auth token",
			Value:       "",
			Type:        components.FieldTypeText,
			Required:    false,
			Help:        "Auth token instead of username/password (optional)",
		},
	}

	form := components.NewForm("Connection Settings", fields)
	form.Focus()

	p := &SettingsPage{
		client: client,
		form:   form,
	}

	if client.IsConnected() {
		url := client.ServerURL()
		if url != "" {
			p.form.SetFieldValue(0, url)
		}
	}

	return p
}

func (p *SettingsPage) SetConnectError(err string) {
	p.connecting = false
	p.form.SetError(err)
}

func (p *SettingsPage) Init() tea.Cmd {
	p.connected = p.client.IsConnected()
	if p.connected {
		p.serverInfo = p.client.ServerName() + " v" + p.client.ServerVersion()
	}
	return nil
}

func (p *SettingsPage) Update(msg tea.Msg) (*SettingsPage, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if cmd, handled := p.form.HandleMouse(msg); handled {
			return p, cmd
		}
		return p, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if p.form.IsCancelFocused() {
				return p, cancelCmd()
			}
			return p.handleConnect()
		}
	}

	_, cmd := p.form.Update(msg)
	return p, cmd
}

func (p *SettingsPage) handleConnect() (*SettingsPage, tea.Cmd) {
	if !p.form.Validate() {
		return p, nil
	}

	if p.connecting {
		return p, nil
	}

	values := p.form.Values()

	url := values["Server URL"]
	username := values["Username"]
	password := values["Password"]
	token := values["Token"]

	p.connecting = true
	p.form.SetError("")

	return p, func() tea.Msg {
		config := &natsclient.Config{
			URL:          url,
			Username:     username,
			Password:     password,
			Token:        token,
			Timeout:      10 * time.Second,
			MaxReconnect: 10,
		}
		newClient := natsclient.NewClient(config)
		err := newClient.Connect()
		return SettingsConnectMsg{Client: newClient, Err: err}
	}
}

type SettingsConnectMsg struct {
	Client *natsclient.Client
	Err    error
}

func (p *SettingsPage) SetSize(width, height int) {
	p.form.SetWidth(width)
	p.form.SetHeight(height)
}

// SetContentOrigin forwards the page-content top-left to the form so it can
// hit-test mouse clicks by absolute coordinates.
func (p *SettingsPage) SetContentOrigin(x, y int) {
	p.originX = x
	p.originY = y
}

func (p *SettingsPage) View() string {
	p.form.SetOrigin(p.originX, p.originY)
	return p.form.View()
}

func (p *SettingsPage) HelpText() string {
	return "↑/↓: Switch fields  Type to edit  Enter: Connect  1-5: Switch Page"
}

func (p *SettingsPage) StatusText() string {
	if p.connecting {
		return "Connecting..."
	}
	if p.connected {
		return "Connected to " + p.client.ServerURL()
	}
	return "Disconnected"
}

// FocusItems returns the arrow-navigable widgets for the Settings page.
// The whole form is one opaque item — arrows delegate to Form's own
// handler and only ↑ at the first field / ↓ at Submit escape to Tabs.
func (p *SettingsPage) FocusItems() []focus.Item {
	return []focus.Item{
		NewFormFocusItem(p.form, func() tea.Cmd {
			if p.form.IsCancelFocused() {
				return cancelCmd()
			}
			_, cmd := p.handleConnect()
			return cmd
		}),
	}
}

// FocusRouted always returns true — the Settings form has no modal state.
func (p *SettingsPage) FocusRouted() bool { return true }
