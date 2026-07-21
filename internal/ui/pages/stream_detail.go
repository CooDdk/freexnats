package pages

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
	"github.com/CooDdk/freexnats/pkg/utils"
)

type StreamDetailPage struct {
	client     *natsclient.Client
	streamName string
	streamInfo *natsclient.StreamInfo
	loading    bool
	err        error
	width      int
	height     int

	tabs      []string
	activeTab int

	consumerTable   *components.TableList
	consumerToolbar *components.Toolbar
	consumers       []*natsclient.ConsumerInfo
	consumersLoaded bool
	consumersLoading bool
	consumersErr    error
}

type StreamLoadedMsg struct {
	Info *natsclient.StreamInfo
	Err  error
}

type StreamConsumersLoadedMsg struct {
	Consumers []*natsclient.ConsumerInfo
	Err       error
}

func NewStreamDetailPage(client *natsclient.Client, streamName string) *StreamDetailPage {
	consumerColumns := []components.Column{
		{Title: "NAME", MinWidth: 18, Flex: 3},
		{Title: "PENDING", MinWidth: 11, Flex: 1},
		{Title: "ACK PENDING", MinWidth: 14},
		{Title: "DELIVERED", MinWidth: 12, Flex: 1},
		{Title: "ACK POLICY", MinWidth: 13},
		{Title: "CREATED", MinWidth: 14, Flex: 2},
	}

	p := &StreamDetailPage{
		client:        client,
		streamName:    streamName,
		tabs:          []string{"Overview", "Messages", "Consumers", "Config"},
		activeTab:     0,
		consumerTable: components.NewTableList(consumerColumns),
	}
	p.consumerTable.SetActionHints([]string{"n: New", "d: Delete", "z: Reset Cursor"})
	p.consumerToolbar = components.NewToolbar("stream-consumers-toolbar", []components.ToolbarAction{
		{ID: "new", Label: "New Consumer", Icon: "+", Primary: true},
		{ID: "refresh", Label: "Refresh"},
	})
	return p
}

func streamDetailConsumerActionKey(id string) string {
	switch id {
	case "new":
		return "n"
	case "refresh":
		return "r"
	}
	return ""
}

func (p *StreamDetailPage) Init() tea.Cmd {
	p.loading = true
	p.err = nil
	return p.loadStreamCmd()
}

func (p *StreamDetailPage) loadStreamCmd() tea.Cmd {
	return func() tea.Msg {
		info, err := p.client.GetStreamInfo(p.streamName)
		return StreamLoadedMsg{Info: info, Err: err}
	}
}

func (p *StreamDetailPage) loadConsumersCmd() tea.Cmd {
	p.consumersLoading = true
	p.consumersErr = nil
	return func() tea.Msg {
		consumers, err := p.client.GetAllConsumerInfos(p.streamName)
		return StreamConsumersLoadedMsg{Consumers: consumers, Err: err}
	}
}

func (p *StreamDetailPage) Update(msg tea.Msg) (*StreamDetailPage, tea.Cmd) {
	switch msg := msg.(type) {
	case StreamLoadedMsg:
		p.loading = false
		if msg.Err != nil {
			p.err = msg.Err
			return p, nil
		}
		p.streamInfo = msg.Info

	case StreamConsumersLoadedMsg:
		p.consumersLoading = false
		if msg.Err != nil {
			p.consumersErr = msg.Err
			return p, nil
		}
		p.consumers = msg.Consumers
		p.consumersLoaded = true
		p.consumerTable.SetRows(p.consumersToRows(msg.Consumers))

	case tea.MouseMsg:
		if p.activeTab == 2 {
			if id, hit := p.consumerToolbar.HandleMouse(msg); hit {
				return p, toolbarKeyCmd(id, streamDetailConsumerActionKey)
			}
			p.consumerTable.HandleMouse(msg)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "l", "right":
			prevTab := p.activeTab
			p.activeTab = (p.activeTab + 1) % len(p.tabs)
			if p.activeTab == 2 && !p.consumersLoaded && !p.consumersLoading {
				return p, p.loadConsumersCmd()
			}
			_ = prevTab
		case "shift+tab", "h", "left":
			p.activeTab = (p.activeTab - 1 + len(p.tabs)) % len(p.tabs)
			if p.activeTab == 2 && !p.consumersLoaded && !p.consumersLoading {
				return p, p.loadConsumersCmd()
			}
		case "r":
			if p.activeTab == 2 {
				return p, p.loadConsumersCmd()
			}
			p.loading = true
			p.err = nil
			return p, p.loadStreamCmd()
		}

		if p.activeTab == 2 && p.consumersLoaded {
			switch msg.String() {
			case "j", "down":
				p.consumerTable.MoveDown()
			case "k", "up":
				p.consumerTable.MoveUp()
			case "g":
				p.consumerTable.GoTop()
			case "G":
				p.consumerTable.GoBottom()
			case "ctrl+d":
				p.consumerTable.MovePageDown()
			case "ctrl+u":
				p.consumerTable.MovePageUp()
			}
		}
	}

	return p, nil
}

func (p *StreamDetailPage) SetSize(width, height int) {
	p.width = width
	p.height = height
	// -10 accounts for header + tabs; -2 for the Consumers-tab toolbar row.
	p.consumerTable.SetSize(width, height-12)
}

// streamDetailConsumersToolbarRelY is the row offset of the Consumers-tab
// toolbar relative to the page's content-column top:
//
//	rows 0..6 : renderHeader (7 lines)
//	row  7    : blank
//	row  8    : tabs
//	row  9    : blank
//	row  10   : consumer toolbar
const streamDetailConsumersToolbarRelY = 10

func (p *StreamDetailPage) SetToolbarOrigin(x, y int) {
	p.consumerToolbar.SetTopLeft(x, y+streamDetailConsumersToolbarRelY)
}

func (p *StreamDetailPage) View() string {
	if p.loading {
		return "  Loading stream info..."
	}
	if p.err != nil {
		return lipgloss.NewStyle().Foreground(ui.Error).Render("  Error: "+p.err.Error()) + "\n\n  Press r to retry"
	}
	if p.streamInfo == nil {
		return "  No stream info"
	}

	tabs := p.renderTabs()
	content := p.renderTabContent()

	return lipgloss.JoinVertical(lipgloss.Left,
		p.renderHeader(),
		"",
		tabs,
		"",
		content,
	)
}

func (p *StreamDetailPage) renderHeader() string {
	w := p.width - 2
	if w < 30 {
		w = 30
	}

	info := p.streamInfo
	topLeft := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Render("\u250c\u2500 ")
	name := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true).Render(info.Name)
	topRight := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Render(" " + strings.Repeat("\u2500", max(0, w-len(" "+info.Name+" ")-4)) + "\u2510")

	labelStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Width(14)
	valueStyle := lipgloss.NewStyle().Foreground(ui.TextColor)

	row := func(l, v string) string {
		return "  " + lipgloss.NewStyle().Foreground(ui.BrandPrimary).Render("\u2502") + "  " + labelStyle.Render(l) + " " + valueStyle.Render(v)
	}

	messages := utils.FormatNumber(info.Messages)
	subjects := ""
	if len(info.Subjects) > 0 {
		subjects = strings.Join(info.Subjects, ", ")
	} else {
		subjects = "-"
	}

	bottomLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandPrimary).Render("\u2514" + strings.Repeat("\u2500", w) + "\u2518")

	return strings.Join([]string{
		"  " + topLeft + name + topRight,
		row("Stream:", info.Name),
		row("Messages:", messages),
		row("Subjects:", subjects),
		row("Storage:", info.Storage),
		row("Created:", utils.FormatAge(info.Created)),
		bottomLine,
	}, "\n")
}

func (p *StreamDetailPage) renderTabs() string {
	var tabs []string
	for i, tab := range p.tabs {
		state := components.ButtonIdle
		if i == p.activeTab {
			state = components.ButtonFocused
		}
		tabs = append(tabs, components.RenderPill(tab, state))
	}
	return "  " + lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

func (p *StreamDetailPage) renderTabContent() string {
	switch p.activeTab {
	case 0:
		return p.renderOverview()
	case 1:
		return p.renderMessages()
	case 2:
		return p.renderConsumersTab()
	case 3:
		return p.renderConfig()
	}
	return ""
}

func (p *StreamDetailPage) renderOverview() string {
	info := p.streamInfo

	stats := [][]string{
		{"Messages", utils.FormatNumber(info.Messages)},
		{"Bytes", utils.FormatBytes(info.Bytes)},
		{"First Sequence", utils.FormatNumber(info.FirstSeq)},
		{"Last Sequence", utils.FormatNumber(info.LastSeq)},
		{"Consumers", utils.FormatNumber(uint64(info.ConsumerCount))},
		{"Created", utils.FormatTime(info.Created)},
		{"Age", utils.FormatAge(info.Created)},
	}

	return p.renderKeyValueTable("Statistics", stats)
}

func (p *StreamDetailPage) renderMessages() string {
	w := p.width - 4
	if w < 30 {
		w = 30
	}

	topLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(
		"\u250c"+strings.Repeat("\u2500", w-2)+"\u2510",
	)
	contentLine := lipgloss.NewStyle().Foreground(ui.SubtleColor).Render(
		"  \u2502  Use j/k keys to browse messages by sequence" +
			strings.Repeat(" ", max(0, w-50)),
	)
	contentLine += lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(" \u2502")

	lastSeqLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2502") +
		"  " + lipgloss.NewStyle().Foreground(ui.SubtleColor).Render("Last Seq: ") +
		lipgloss.NewStyle().Foreground(ui.TextColor).Render(utils.FormatNumber(p.streamInfo.LastSeq))
	padLen := w - 2 - lipgloss.Width("  Last Seq: "+utils.FormatNumber(p.streamInfo.LastSeq))
	if padLen < 0 {
		padLen = 0
	}
	lastSeqLine += strings.Repeat(" ", padLen) + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2502")

	bottomLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(
		"\u2514"+strings.Repeat("\u2500", w-2)+"\u2518",
	)

	return strings.Join([]string{topLine, contentLine, lastSeqLine, bottomLine}, "\n")
}

func (p *StreamDetailPage) renderConsumersTab() string {
	toolbar := p.consumerToolbar.View()
	var body string
	switch {
	case p.consumersLoading:
		body = "  Loading consumers..."
	case p.consumersErr != nil:
		body = "  Error loading consumers: " + p.consumersErr.Error()
	case !p.consumersLoaded:
		body = "  Press r to load consumers..."
	default:
		body = p.consumerTable.View()
	}
	return toolbar + "\n\n" + body
}

func (p *StreamDetailPage) renderConfig() string {
	info := p.streamInfo

	config := [][]string{
		{"Name", info.Name},
		{"Subjects", strings.Join(info.Subjects, ", ")},
		{"Storage", info.Storage},
		{"Replicas", utils.FormatNumber(uint64(info.Replicas))},
		{"Max Messages", p.formatMaxValue(info.MaxMsgs)},
		{"Max Bytes", p.formatMaxBytes(info.MaxBytes)},
		{"Max Age", p.formatMaxAge(info.MaxAge)},
		{"Description", info.Description},
	}

	return p.renderKeyValueTable("Configuration", config)
}

func (p *StreamDetailPage) renderKeyValueTable(title string, rows [][]string) string {
	w := p.width - 4
	if w < 30 {
		w = 30
	}

	var lines []string

	topLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(
		"\u250c\u2500 ") + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Bold(true).Render(title) +
		" " + strings.Repeat("\u2500", max(0, w-len(title)-5)) +
		lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2510")
	lines = append(lines, topLine)

	for _, row := range rows {
		key := lipgloss.NewStyle().
			Foreground(ui.SubtleColor).
			Width(18).
			Render(row[0])
		val := lipgloss.NewStyle().
			Foreground(ui.TextColor).
			Render(row[1])

		content := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2502") +
			"  " + key + "  " + val
		padLen := w - 2 - lipgloss.Width("  "+key+"  "+val)
		if padLen < 0 {
			padLen = 0
		}
		content += strings.Repeat(" ", padLen) + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2502")
		lines = append(lines, content)
	}

	bottomLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(
		"\u2514"+strings.Repeat("\u2500", w-2)+"\u2518")
	lines = append(lines, bottomLine)

	return strings.Join(lines, "\n")
}

func (p *StreamDetailPage) consumersToRows(consumers []*natsclient.ConsumerInfo) [][]string {
	rows := make([][]string, len(consumers))
	for i, c := range consumers {
		rows[i] = []string{
			" " + c.Name,
			" " + utils.FormatNumber(c.NumPending),
			" " + utils.FormatNumber(uint64(c.NumAckPending)),
			" " + utils.FormatNumber(c.Delivered),
			" " + c.AckPolicy,
			" " + utils.FormatAge(c.Created),
		}
	}
	return rows
}

func (p *StreamDetailPage) formatMaxValue(v int64) string {
	if v < 0 {
		return "unlimited"
	}
	return utils.FormatNumber(uint64(v))
}

func (p *StreamDetailPage) formatMaxBytes(v int64) string {
	if v < 0 {
		return "unlimited"
	}
	return utils.FormatBytes(uint64(v))
}

func (p *StreamDetailPage) formatMaxAge(d time.Duration) string {
	if d == 0 {
		return "unlimited"
	}
	return utils.FormatDuration(d)
}

func (p *StreamDetailPage) ActiveTab() int {
	return p.activeTab
}

func (p *StreamDetailPage) StreamName() string {
	return p.streamName
}

func (p *StreamDetailPage) HelpText() string {
	if p.activeTab == 2 && p.consumersLoaded {
		return "Tab: Switch Tab  j/k: Navigate  n: New  r: Refresh  Esc: Back"
	}
	return "Tab: Switch Tab  r: Refresh  Esc: Back"
}

func (p *StreamDetailPage) StatusText() string {
	if p.loading {
		return "Loading..."
	}
	if p.activeTab == 2 && p.consumersLoaded {
		return p.streamName + " - Consumers: " + p.consumerTable.PositionText()
	}
	return p.streamName + " - " + p.tabs[p.activeTab]
}

// streamDetailTabsFocusItem is a lightweight focus item over the sub-tab
// strip. Left/Right cycle p.activeTab and lazy-load the Consumers tab; the
// item is a no-op on Focus/Blur/Enter because tab visuals already show the
// active tab.
type streamDetailTabsFocusItem struct {
	page *StreamDetailPage
}

func (s *streamDetailTabsFocusItem) Focus() {}
func (s *streamDetailTabsFocusItem) Blur()  {}
func (s *streamDetailTabsFocusItem) Activate() tea.Cmd { return nil }

func (s *streamDetailTabsFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	switch dir {
	case focus.DirLeft:
		if s.page.activeTab == 0 {
			return nil, true // absorb — no wrap
		}
		s.page.activeTab--
		if s.page.activeTab == 2 && !s.page.consumersLoaded && !s.page.consumersLoading {
			return s.page.loadConsumersCmd(), true
		}
		return nil, true
	case focus.DirRight:
		if s.page.activeTab == len(s.page.tabs)-1 {
			return nil, true
		}
		s.page.activeTab++
		if s.page.activeTab == 2 && !s.page.consumersLoaded && !s.page.consumersLoading {
			return s.page.loadConsumersCmd(), true
		}
		return nil, true
	case focus.DirUp:
		return nil, false
	case focus.DirDown:
		return nil, false
	}
	return nil, false
}

// FocusItems returns the arrow-navigable widgets for the current sub-tab.
// The sub-tab strip is always present; the Consumers tab additionally
// contributes its toolbar + table so ↓ descends into the row list and ↑
// climbs back to the tab strip.
func (p *StreamDetailPage) FocusItems() []focus.Item {
	items := []focus.Item{&streamDetailTabsFocusItem{page: p}}
	if p.activeTab == 2 {
		items = append(items,
			NewToolbarFocusItem(p.consumerToolbar, func(id string) tea.Cmd {
				return toolbarKeyCmd(id, streamDetailConsumerActionKey)
			}),
			NewTableListFocusItem(p.consumerTable, func() tea.Cmd { return nil }),
		)
	}
	return items
}

// FocusRouted is always true — StreamDetail has no modal state that
// intercepts arrow keys.
func (p *StreamDetailPage) FocusRouted() bool { return true }
