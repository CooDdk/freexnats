package pages

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
	"github.com/CooDdk/freexnats/pkg/utils"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
)

type StreamsPage struct {
	table    *components.TableList
	overlay  *components.DetailOverlay
	toolbar  *components.Toolbar
	client   *natsclient.Client
	streams  []*natsclient.StreamInfo
	loading  bool
	err      error
	width    int
	height   int
	canWrite bool
}

type StreamsLoadedMsg struct {
	Streams []*natsclient.StreamInfo
	Err     error
}

func NewStreamsPage(client *natsclient.Client) *StreamsPage {
	columns := []components.Column{
		{Title: "NAME", MinWidth: 18, Flex: 3},
		{Title: "MESSAGES", MinWidth: 12, Flex: 1},
		{Title: "BYTES", MinWidth: 12, Flex: 1},
		{Title: "CONSUMERS", MinWidth: 13, Flex: 1},
		{Title: "STORAGE", MinWidth: 11},
		{Title: "CREATED", MinWidth: 14, Flex: 2},
	}

	p := &StreamsPage{
		table:   components.NewTableList(columns),
		overlay: components.NewDetailOverlay(),
		client:  client,
	}
	p.table.SetActionHints([]string{"Enter: Details", "v: Peek", "e: Edit", "d: Delete"})
	p.toolbar = components.NewToolbar("streams-toolbar", p.buildActions())
	return p
}

// SetConnected updates the toolbar's disabled state so write actions are
// greyed out when NATS is not connected.
func (p *StreamsPage) SetConnected(ok bool) {
	p.canWrite = ok
	p.toolbar.SetActions(p.buildActions())
}

func (p *StreamsPage) buildActions() []components.ToolbarAction {
	return []components.ToolbarAction{
		{ID: "new", Label: "New Stream", Icon: "+", Primary: true, Disabled: !p.canWrite},
		{ID: "edit", Label: "Edit", Disabled: !p.canWrite},
		{ID: "purge", Label: "Purge", Disabled: !p.canWrite},
		{ID: "delete", Label: "Delete", Disabled: !p.canWrite},
		{ID: "refresh", Label: "Refresh"},
	}
}

func (p *StreamsPage) Init() tea.Cmd {
	p.loading = true
	p.err = nil
	return p.loadStreamsCmd()
}

func (p *StreamsPage) loadStreamsCmd() tea.Cmd {
	return func() tea.Msg {
		streams, err := p.client.GetAllStreamInfos()
		return StreamsLoadedMsg{Streams: streams, Err: err}
	}
}

func (p *StreamsPage) Update(msg tea.Msg) (*StreamsPage, tea.Cmd) {
	switch msg := msg.(type) {
	case StreamsLoadedMsg:
		p.loading = false
		if msg.Err != nil {
			p.err = msg.Err
			return p, nil
		}
		p.streams = msg.Streams
		p.table.SetRows(p.streamsToRows(msg.Streams))

	case tea.MouseMsg:
		if p.overlay.Visible() {
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				p.overlay.Hide()
			}
			return p, nil
		}
		if id, hit := p.toolbar.HandleMouse(msg); hit {
			return p, toolbarKeyCmd(id, streamActionKey)
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
		case "v":
			p.showOverlay()
		case "r":
			p.loading = true
			p.err = nil
			return p, p.loadStreamsCmd()
		}
	}

	return p, nil
}

// streamActionKey maps a toolbar action ID to the equivalent keyboard key,
// so clicking a button and pressing the shortcut end up in the same code path.
func streamActionKey(id string) string {
	switch id {
	case "new":
		return "n"
	case "edit":
		return "e"
	case "purge":
		return "p"
	case "delete":
		return "d"
	case "refresh":
		return "r"
	}
	return ""
}

func (p *StreamsPage) showOverlay() {
	s := p.SelectedStream()
	if s == nil {
		return
	}
	subjects := strings.Join(s.Subjects, ", ")
	if subjects == "" {
		subjects = "-"
	}
	rows := []components.DetailRow{
		{Label: "Name", Value: s.Name},
		{Label: "Subjects", Value: subjects},
		{Label: "Storage", Value: s.Storage},
		{Label: "Replicas", Value: utils.FormatNumber(uint64(s.Replicas))},
		{Label: "Messages", Value: utils.FormatNumber(s.Messages)},
		{Label: "Bytes", Value: utils.FormatBytes(s.Bytes)},
		{Label: "First Seq", Value: utils.FormatNumber(s.FirstSeq)},
		{Label: "Last Seq", Value: utils.FormatNumber(s.LastSeq)},
		{Label: "Consumers", Value: utils.FormatNumber(uint64(s.ConsumerCount))},
		{Label: "Max Msgs", Value: formatMax(s.MaxMsgs)},
		{Label: "Max Bytes", Value: formatMaxBytes(s.MaxBytes)},
		{Label: "Max Age", Value: formatMaxAge(s.MaxAge)},
		{Label: "Description", Value: emptyDash(s.Description)},
		{Label: "Created", Value: utils.FormatTime(s.Created)},
		{Label: "Age", Value: utils.FormatAge(s.Created)},
	}
	p.overlay.Show("Stream: "+s.Name, rows)
}

const streamsToolbarRows = 2 // toolbar row + blank line

func (p *StreamsPage) SetSize(width, height int) {
	p.width = width
	p.height = height
	tableH := height - streamsToolbarRows
	if tableH < 3 {
		tableH = 3
	}
	p.table.SetSize(width, tableH)
}

// SetToolbarOrigin tells the toolbar its absolute (x, y) in the final view
// so coordinate-based hit testing works despite outer lipgloss processing.
func (p *StreamsPage) SetToolbarOrigin(x, y int) {
	p.toolbar.SetTopLeft(x, y)
}

// FocusItems returns the ordered list of keyboard-focusable regions on
// this page for focus.Manager to walk. Tabs is added at the app level
// (index 0) so pages return only their own content-area chrome.
//
// activateRow returns the Cmd that should run when Enter is pressed on
// the currently selected table row (typically "open detail"). It's
// injected by app.go because the page doesn't know about the app-level
// view mode machinery.
func (p *StreamsPage) FocusItems(activateRow func() tea.Cmd) []focus.Item {
	return []focus.Item{
		NewToolbarFocusItem(p.toolbar, func(id string) tea.Cmd {
			return toolbarKeyCmd(id, streamActionKey)
		}),
		NewTableListFocusItem(p.table, activateRow),
	}
}

// Toolbar exposes the underlying toolbar so the app-level focus manager
// can query action IDs at the currently keyboard-focused button. The
// widget itself is not modified externally.
func (p *StreamsPage) Toolbar() *components.Toolbar { return p.toolbar }

// Table exposes the underlying table for the same reason as Toolbar.
func (p *StreamsPage) Table() *components.TableList { return p.table }

func (p *StreamsPage) View() string {
	if p.loading {
		return "  Loading streams..."
	}
	if p.err != nil {
		return "  Error: " + p.err.Error()
	}
	content := p.toolbar.View() + "\n\n" + p.table.View()
	return p.overlay.PlaceOn(content, p.width, p.height)
}

func (p *StreamsPage) HelpText() string {
	if p.overlay.Visible() {
		return "Esc: Close"
	}
	return "j/k: Navigate  g/G: Top/Bottom  r: Refresh  Enter: Details  v: Peek  n: New  e: Edit  p: Purge  d: Delete"
}

func (p *StreamsPage) StatusText() string {
	if p.loading {
		return "Loading..."
	}
	return "Streams: " + p.table.PositionText()
}

func (p *StreamsPage) streamsToRows(streams []*natsclient.StreamInfo) [][]string {
	rows := make([][]string, len(streams))
	for i, s := range streams {
		rows[i] = []string{
			" " + s.Name,
			" " + utils.FormatNumber(s.Messages),
			" " + utils.FormatBytes(s.Bytes),
			" " + utils.FormatNumber(uint64(s.ConsumerCount)),
			" " + s.Storage,
			" " + utils.FormatAge(s.Created),
		}
	}
	return rows
}

func (p *StreamsPage) SelectedStream() *natsclient.StreamInfo {
	idx := p.table.SelectedIndex()
	if idx < 0 || idx >= len(p.streams) {
		return nil
	}
	return p.streams[idx]
}
