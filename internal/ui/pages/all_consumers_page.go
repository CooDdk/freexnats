package pages

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
	"github.com/CooDdk/freexnats/pkg/utils"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
)

type AllConsumerEntry struct {
	StreamName   string
	ConsumerInfo *natsclient.ConsumerInfo
}

type AllConsumersPage struct {
	table    *components.TableList
	overlay  *components.DetailOverlay
	toolbar  *components.Toolbar
	client   *natsclient.Client
	entries  []*AllConsumerEntry
	loading  bool
	err      error
	width    int
	height   int
	canWrite bool
}

type AllConsumersLoadedMsg struct {
	Entries []*AllConsumerEntry
	Err     error
}

func NewAllConsumersPage(client *natsclient.Client) *AllConsumersPage {
	columns := []components.Column{
		{Title: "STREAM", MinWidth: 12, Flex: 1},
		{Title: "NAME", MinWidth: 18, Flex: 3},
		{Title: "PENDING", MinWidth: 12, Flex: 1},
		{Title: "ACK PEND", MinWidth: 11, Flex: 1},
		{Title: "REDELIV", MinWidth: 10, Flex: 0},
		{Title: "ACK FLOOR", MinWidth: 12, Flex: 0},
		{Title: "POLICY", MinWidth: 10, Flex: 0},
		{Title: "AGE", MinWidth: 10, Flex: 1},
	}

	p := &AllConsumersPage{
		table:   components.NewTableList(columns),
		overlay: components.NewDetailOverlay(),
		client:  client,
	}
	p.table.SetActionHints([]string{"Enter: Details", "d: Delete", "z: Reset Cursor"})
	p.toolbar = components.NewToolbar("all-consumers-toolbar", p.buildActions())
	return p
}

// SetConnected toggles the "not connected" grey-out on write actions.
func (p *AllConsumersPage) SetConnected(ok bool) {
	p.canWrite = ok
	p.toolbar.SetActions(p.buildActions())
}

func (p *AllConsumersPage) buildActions() []components.ToolbarAction {
	return []components.ToolbarAction{
		{ID: "reset", Label: "Reset Cursor", Disabled: !p.canWrite},
		{ID: "delete", Label: "Delete", Disabled: !p.canWrite},
		{ID: "refresh", Label: "Refresh"},
	}
}

func consumersActionKey(id string) string {
	switch id {
	case "refresh":
		return "r"
	case "delete":
		return "d"
	case "reset":
		return "z"
	}
	return ""
}

func (p *AllConsumersPage) Init() tea.Cmd {
	p.loading = true
	p.err = nil
	return p.loadAllConsumersCmd()
}

func (p *AllConsumersPage) loadAllConsumersCmd() tea.Cmd {
	return func() tea.Msg {
		streams, err := p.client.GetAllStreamInfos()
		if err != nil {
			return AllConsumersLoadedMsg{Err: err}
		}

		var entries []*AllConsumerEntry
		for _, stream := range streams {
			consumers, err := p.client.GetAllConsumerInfos(stream.Name)
			if err != nil {
				continue
			}
			for _, c := range consumers {
				entries = append(entries, &AllConsumerEntry{
					StreamName:   stream.Name,
					ConsumerInfo: c,
				})
			}
		}

		return AllConsumersLoadedMsg{Entries: entries}
	}
}

func (p *AllConsumersPage) Update(msg tea.Msg) (*AllConsumersPage, tea.Cmd) {
	switch msg := msg.(type) {
	case AllConsumersLoadedMsg:
		p.loading = false
		if msg.Err != nil {
			p.err = msg.Err
			return p, nil
		}
		p.entries = msg.Entries
		p.table.SetRows(p.entriesToRows(msg.Entries))

	case tea.MouseMsg:
		if p.overlay.Visible() {
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				p.overlay.Hide()
			}
			return p, nil
		}
		if id, hit := p.toolbar.HandleMouse(msg); hit {
			return p, toolbarKeyCmd(id, consumersActionKey)
		}
		if p.table.HandleMouse(msg) == components.MouseRowClicked {
			p.showOverlay()
		}

	case tea.KeyMsg:
		if p.overlay.Visible() {
			if msg.String() == "esc" || msg.String() == "v" || msg.String() == "enter" {
				p.overlay.Hide()
			}
			return p, nil
		}
		switch msg.String() {
		case "j", "down":
			p.table.MoveDown()
		case "k", "up":
			p.table.MoveUp()
		case "g":
			p.table.GoTop()
		case "G":
			p.table.GoBottom()
		case "ctrl+d":
			p.table.MovePageDown()
		case "ctrl+u":
			p.table.MovePageUp()
		case "v", "enter":
			p.showOverlay()
		case "r":
			p.loading = true
			p.err = nil
			return p, p.loadAllConsumersCmd()
		}
	}

	return p, nil
}

func (p *AllConsumersPage) showOverlay() {
	idx := p.table.SelectedIndex()
	if idx < 0 || idx >= len(p.entries) {
		return
	}
	e := p.entries[idx]
	c := e.ConsumerInfo
	rows := []components.DetailRow{
		{Label: "Stream", Value: e.StreamName},
		{Label: "Name", Value: c.Name},
		{Label: "Durable", Value: emptyDash(c.DurableName)},
		{Label: "Description", Value: emptyDash(c.Description)},
		{Label: "Filter Subject", Value: emptyDash(c.FilterSubject)},
		{Label: "Ack Policy", Value: c.AckPolicy},
		{Label: "Deliver Policy", Value: c.DeliverPolicy},
		{Label: "Replay Policy", Value: c.ReplayPolicy},
		{Label: "Max Deliver", Value: formatMax(int64(c.MaxDeliver))},
		{Label: "Delivered", Value: utils.FormatNumber(c.Delivered)},
		{Label: "Ack Floor", Value: utils.FormatNumber(c.AckFloorStream)},
		{Label: "Pending", Value: utils.FormatNumber(c.NumPending)},
		{Label: "Ack Pending", Value: utils.FormatNumber(uint64(c.NumAckPending))},
		{Label: "Redelivered", Value: utils.FormatNumber(uint64(c.NumRedelivered))},
		{Label: "Created", Value: utils.FormatTime(c.Created)},
		{Label: "Age", Value: utils.FormatAge(c.Created)},
	}
	p.overlay.Show("Consumer: "+c.Name, rows)
}

const allConsumersToolbarRows = 2 // toolbar row + blank line

func (p *AllConsumersPage) SetSize(width, height int) {
	p.width = width
	p.height = height
	tableH := height - allConsumersToolbarRows
	if tableH < 3 {
		tableH = 3
	}
	p.table.SetSize(width, tableH)
}

func (p *AllConsumersPage) SetToolbarOrigin(x, y int) {
	p.toolbar.SetTopLeft(x, y)
}

// FocusItems mirrors StreamsPage's pattern — Toolbar first, then TableList.
// activateRow is what runs on Enter over a row (typically "show overlay",
// injected by app.go).
func (p *AllConsumersPage) FocusItems(activateRow func() tea.Cmd) []focus.Item {
	return []focus.Item{
		NewToolbarFocusItem(p.toolbar, func(id string) tea.Cmd {
			return toolbarKeyCmd(id, consumersActionKey)
		}),
		NewTableListFocusItem(p.table, activateRow),
	}
}

// Toolbar / Table expose the widgets so the app-level focus manager can
// query action IDs / cursor state without going through Update.
func (p *AllConsumersPage) Toolbar() *components.Toolbar   { return p.toolbar }
func (p *AllConsumersPage) Table() *components.TableList   { return p.table }

// FocusRouted reports whether arrow keys should flow through the focus
// manager on this page right now. Overlay-open state disables routing so
// the overlay's own key handler (Esc/v/Enter to close) still works.
func (p *AllConsumersPage) FocusRouted() bool { return !p.overlay.Visible() }

// ShowOverlay is the exported version of showOverlay so app.go's Enter
// activation callback can trigger it without duplicating the code.
func (p *AllConsumersPage) ShowOverlay() { p.showOverlay() }

// SelectedEntry returns the currently highlighted consumer entry, or nil
// when the table is empty. Used by app.go to snapshot the target of
// destructive/mutating operations at the moment the confirm dialog fires.
func (p *AllConsumersPage) SelectedEntry() *AllConsumerEntry {
	idx := p.table.SelectedIndex()
	if idx < 0 || idx >= len(p.entries) {
		return nil
	}
	return p.entries[idx]
}

func (p *AllConsumersPage) View() string {
	if p.loading {
		return "  Loading consumers..."
	}
	if p.err != nil {
		return "  Error: " + p.err.Error()
	}
	content := p.toolbar.View() + "\n\n" + p.table.View()
	return p.overlay.PlaceOn(content, p.width, p.height)
}

func (p *AllConsumersPage) HelpText() string {
	if p.overlay.Visible() {
		return "Esc: Close"
	}
	return "j/k: Navigate  g/G: Top/Bottom  r: Refresh  Enter/v: Details  d: Delete  z: Reset Cursor  Ctrl+D/U: Page"
}

func (p *AllConsumersPage) StatusText() string {
	return "All Consumers: " + p.table.PositionText()
}

func (p *AllConsumersPage) entriesToRows(entries []*AllConsumerEntry) [][]string {
	rows := make([][]string, len(entries))
	for i, e := range entries {
		c := e.ConsumerInfo
		rows[i] = []string{
			" " + e.StreamName,
			" " + c.Name,
			" " + pendingCell(c.NumPending),
			" " + utils.FormatNumber(uint64(c.NumAckPending)),
			" " + redeliveredCell(c.NumRedelivered),
			" " + utils.FormatNumber(c.AckFloorStream),
			" " + c.AckPolicy,
			" " + utils.FormatAge(c.Created),
		}
	}
	return rows
}

// pendingCell renders the pending count with a severity-colored dot prefix.
// Foreground-only styling on the dot keeps the outer row background intact
// under lipgloss composition.
func pendingCell(pending uint64) string {
	var color lipgloss.TerminalColor
	switch {
	case pending == 0:
		color = ui.Success
	case pending <= 1000:
		color = ui.Warning
	default:
		color = ui.Error
	}
	dot := lipgloss.NewStyle().Foreground(color).Bold(true).Render("●")
	return dot + " " + utils.FormatNumber(pending)
}

// redeliveredCell colors a nonzero count in warning yellow so redelivery
// pressure is visible at a glance without adding a separate status column.
func redeliveredCell(n int) string {
	num := utils.FormatNumber(uint64(n))
	if n == 0 {
		return num
	}
	return lipgloss.NewStyle().Foreground(ui.Warning).Bold(true).Render(num)
}
