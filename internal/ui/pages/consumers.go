package pages

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/pkg/utils"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
)

type ConsumersPage struct {
	table      *components.TableList
	client     *natsclient.Client
	streamName string
	consumers  []*natsclient.ConsumerInfo
	loading    bool
	err        error
}

type ConsumersLoadedMsg struct {
	Consumers []*natsclient.ConsumerInfo
	Err       error
}

func NewConsumersPage(client *natsclient.Client, streamName string) *ConsumersPage {
	columns := []components.Column{
		{Title: "NAME", MinWidth: 18, Flex: 3},
		{Title: "PENDING", MinWidth: 11, Flex: 1},
		{Title: "ACK PENDING", MinWidth: 14, Flex: 1},
		{Title: "DELIVERED", MinWidth: 12, Flex: 1},
		{Title: "ACK POLICY", MinWidth: 13},
		{Title: "CREATED", MinWidth: 14, Flex: 2},
	}

	return &ConsumersPage{
		table:      components.NewTableList(columns),
		client:     client,
		streamName: streamName,
	}
}

func (p *ConsumersPage) Init() tea.Cmd {
	return p.loadConsumersCmd()
}

func (p *ConsumersPage) loadConsumersCmd() tea.Cmd {
	return func() tea.Msg {
		consumers, err := p.client.GetAllConsumerInfos(p.streamName)
		return ConsumersLoadedMsg{Consumers: consumers, Err: err}
	}
}

func (p *ConsumersPage) Update(msg tea.Msg) (*ConsumersPage, tea.Cmd) {
	switch msg := msg.(type) {
	case ConsumersLoadedMsg:
		p.loading = false
		if msg.Err != nil {
			p.err = msg.Err
			return p, nil
		}
		p.consumers = msg.Consumers
		p.table.SetRows(p.consumersToRows(msg.Consumers))

	case tea.KeyMsg:
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
		case "r":
			p.loading = true
			p.err = nil
			return p, p.loadConsumersCmd()
		}
	}

	return p, nil
}

func (p *ConsumersPage) SetSize(width, height int) {
	p.table.SetSize(width, height)
}

func (p *ConsumersPage) View() string {
	if p.loading {
		return "  Loading consumers..."
	}
	if p.err != nil {
		return "  Error: " + p.err.Error()
	}
	return p.table.View()
}

func (p *ConsumersPage) HelpText() string {
	return "j/k: Navigate  g/G: Top/Bottom  r: Refresh  d: Delete"
}

func (p *ConsumersPage) StatusText() string {
	if p.loading {
		return "Loading..."
	}
	return "Consumers: " + p.table.PositionText() + " (stream: " + p.streamName + ")"
}

func (p *ConsumersPage) consumersToRows(consumers []*natsclient.ConsumerInfo) [][]string {
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

func (p *ConsumersPage) SelectedConsumer() *natsclient.ConsumerInfo {
	idx := p.table.SelectedIndex()
	if idx < 0 || idx >= len(p.consumers) {
		return nil
	}
	return p.consumers[idx]
}

func (p *ConsumersPage) SetStreamName(name string) {
	p.streamName = name
}
