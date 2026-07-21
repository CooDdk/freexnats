package pages

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
	"github.com/CooDdk/freexnats/pkg/utils"
)

type KVStoreMode int

const (
	ModeBucketList KVStoreMode = iota
	ModeKeyList
	ModeValueView
)

type KVStorePage struct {
	client *natsclient.Client
	mode   KVStoreMode
	width  int
	height int

	// Bucket list
	bucketTable *components.TableList
	bucketOverlay *components.DetailOverlay
	bucketToolbar *components.Toolbar
	buckets     []*natsclient.BucketInfo
	bucketLoading bool
	bucketErr   error

	// Key list
	keyTable    *components.TableList
	keyOverlay  *components.DetailOverlay
	keyToolbar  *components.Toolbar
	keys        []string
	selectedBucket string
	keyLoading  bool
	keyErr      error

	// Value view
	entry       *natsclient.KVEntry
	selectedKey string
	valueLoading bool
	valueErr    error

	// Delete confirmation
	deleteConfirm bool
	deleteLoading bool
	deleteErr    error

	canWrite bool
}

// Messages

type KVBucketsLoadedMsg struct {
	Buckets []*natsclient.BucketInfo
	Err     error
}

type KVKeysLoadedMsg struct {
	Keys []string
	Err  error
}

type KVValueLoadedMsg struct {
	Entry *natsclient.KVEntry
	Err   error
}

type KVKeyDeletedMsg struct {
	Err error
}

func NewKVStorePage(client *natsclient.Client) *KVStorePage {
	bucketColumns := []components.Column{
		{Title: "NAME", MinWidth: 18, Flex: 3},
		{Title: "VALUES", MinWidth: 11, Flex: 1},
		{Title: "HISTORY", MinWidth: 11, Flex: 1},
		{Title: "TTL", MinWidth: 12},
		{Title: "BYTES", MinWidth: 12, Flex: 1},
		{Title: "CREATED", MinWidth: 14, Flex: 2},
	}

	keyColumns := []components.Column{
		{Title: "KEY", MinWidth: 22, Flex: 4},
		{Title: "REVISION", MinWidth: 12, Flex: 1},
		{Title: "CREATED", MinWidth: 14, Flex: 2},
	}

	p := &KVStorePage{
		client:        client,
		mode:          ModeBucketList,
		bucketTable:   components.NewTableList(bucketColumns),
		keyTable:      components.NewTableList(keyColumns),
		bucketOverlay: components.NewDetailOverlay(),
		keyOverlay:    components.NewDetailOverlay(),
	}
	p.bucketTable.SetActionHints([]string{"Enter: Open", "v: Peek", "d: Delete"})
	p.keyTable.SetActionHints([]string{"Enter: View", "e: Edit", "d: Delete"})
	p.bucketToolbar = components.NewToolbar("kv-buckets-toolbar", p.buildBucketActions())
	p.keyToolbar = components.NewToolbar("kv-keys-toolbar", p.buildKeyActions())
	return p
}

// SetConnected toggles the "not connected" grey-out on write actions.
func (p *KVStorePage) SetConnected(ok bool) {
	p.canWrite = ok
	p.bucketToolbar.SetActions(p.buildBucketActions())
	p.keyToolbar.SetActions(p.buildKeyActions())
}

func (p *KVStorePage) buildBucketActions() []components.ToolbarAction {
	return []components.ToolbarAction{
		{ID: "new", Label: "New Bucket", Disabled: !p.canWrite},
		{ID: "delete", Label: "Delete", Disabled: !p.canWrite},
		{ID: "refresh", Label: "Refresh"},
	}
}

func (p *KVStorePage) buildKeyActions() []components.ToolbarAction {
	return []components.ToolbarAction{
		{ID: "new", Label: "New Key", Disabled: !p.canWrite},
		{ID: "delete", Label: "Delete", Disabled: !p.canWrite},
		{ID: "refresh", Label: "Refresh"},
	}
}

func kvActionKey(id string) string {
	switch id {
	case "refresh":
		return "r"
	case "new":
		return "n"
	case "delete":
		return "d"
	}
	return ""
}

func (p *KVStorePage) Init() tea.Cmd {
	return p.loadBucketsCmd()
}

// Load commands

func (p *KVStorePage) loadBucketsCmd() tea.Cmd {
	p.bucketLoading = true
	p.bucketErr = nil
	return func() tea.Msg {
		buckets, err := p.client.ListKVBuckets()
		return KVBucketsLoadedMsg{Buckets: buckets, Err: err}
	}
}

func (p *KVStorePage) loadKeysCmd() tea.Cmd {
	p.keyLoading = true
	p.keyErr = nil
	return func() tea.Msg {
		keys, err := p.client.ListKVKeys(p.selectedBucket)
		return KVKeysLoadedMsg{Keys: keys, Err: err}
	}
}

func (p *KVStorePage) loadValueCmd() tea.Cmd {
	p.valueLoading = true
	p.valueErr = nil
	return func() tea.Msg {
		entry, err := p.client.GetKVValue(p.selectedBucket, p.selectedKey)
		return KVValueLoadedMsg{Entry: entry, Err: err}
	}
}

func (p *KVStorePage) deleteKeyCmd() tea.Cmd {
	p.deleteLoading = true
	p.deleteErr = nil
	return func() tea.Msg {
		err := p.client.DeleteKVKey(p.selectedBucket, p.selectedKey)
		return KVKeyDeletedMsg{Err: err}
	}
}

// Update

func (p *KVStorePage) Update(msg tea.Msg) (*KVStorePage, tea.Cmd) {
	switch msg := msg.(type) {
	case KVBucketsLoadedMsg:
		p.bucketLoading = false
		if msg.Err != nil {
			p.bucketErr = msg.Err
			return p, nil
		}
		p.buckets = msg.Buckets
		p.bucketTable.SetRows(p.bucketsToRows(msg.Buckets))

	case KVKeysLoadedMsg:
		p.keyLoading = false
		if msg.Err != nil {
			p.keyErr = msg.Err
			return p, nil
		}
		p.keys = msg.Keys
		p.keyTable.SetRows(p.keysToRows(msg.Keys))

	case KVValueLoadedMsg:
		p.valueLoading = false
		if msg.Err != nil {
			p.valueErr = msg.Err
			return p, nil
		}
		p.entry = msg.Entry

	case KVKeyDeletedMsg:
		p.deleteLoading = false
		p.deleteConfirm = false
		if msg.Err != nil {
			p.deleteErr = msg.Err
			return p, nil
		}
		// After deletion, go back to key list and refresh
		p.mode = ModeKeyList
		p.entry = nil
		p.deleteErr = nil
		return p, p.loadKeysCmd()

	case tea.MouseMsg:
		switch p.mode {
		case ModeBucketList:
			if p.bucketOverlay.Visible() {
				if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
					p.bucketOverlay.Hide()
				}
				return p, nil
			}
			if id, hit := p.bucketToolbar.HandleMouse(msg); hit {
				return p, toolbarKeyCmd(id, kvActionKey)
			}
			if p.bucketTable.HandleMouse(msg) == components.MouseRowClicked {
				p.showBucketOverlay()
			}
		case ModeKeyList:
			if p.keyOverlay.Visible() {
				if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
					p.keyOverlay.Hide()
				}
				return p, nil
			}
			if id, hit := p.keyToolbar.HandleMouse(msg); hit {
				return p, toolbarKeyCmd(id, kvActionKey)
			}
			if p.keyTable.HandleMouse(msg) == components.MouseRowClicked {
				p.showKeyOverlay()
			}
		}

	case tea.KeyMsg:
		if p.deleteConfirm {
			return p.handleDeleteConfirm(msg)
		}
		return p.handleKeyMsg(msg)
	}

	return p, nil
}

func (p *KVStorePage) handleDeleteConfirm(msg tea.KeyMsg) (*KVStorePage, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return p, p.deleteKeyCmd()
	case "esc":
		p.deleteConfirm = false
		p.deleteErr = nil
	}
	return p, nil
}

func (p *KVStorePage) handleKeyMsg(msg tea.Msg) (*KVStorePage, tea.Cmd) {
	keyStr := ""
	if km, ok := msg.(tea.KeyMsg); ok {
		keyStr = km.String()
	} else {
		return p, nil
	}

	switch p.mode {
	case ModeBucketList:
		if p.bucketOverlay.Visible() {
			if keyStr == "esc" || keyStr == "v" {
				p.bucketOverlay.Hide()
			}
			return p, nil
		}
		return p.handleBucketListKeys(keyStr)
	case ModeKeyList:
		if p.keyOverlay.Visible() {
			if keyStr == "esc" || keyStr == "v" {
				p.keyOverlay.Hide()
			}
			return p, nil
		}
		return p.handleKeyListKeys(keyStr)
	case ModeValueView:
		return p.handleValueViewKeys(keyStr)
	}
	return p, nil
}

func (p *KVStorePage) handleBucketListKeys(key string) (*KVStorePage, tea.Cmd) {
	switch key {
	case "j", "down":
		p.bucketTable.MoveDown()
	case "k", "up":
		p.bucketTable.MoveUp()
	case "g":
		p.bucketTable.GoTop()
	case "G":
		p.bucketTable.GoBottom()
	case "ctrl+d":
		p.bucketTable.MovePageDown()
	case "ctrl+u":
		p.bucketTable.MovePageUp()
	case "v":
		p.showBucketOverlay()
	case "r":
		return p, p.loadBucketsCmd()
	case "enter":
		idx := p.bucketTable.SelectedIndex()
		if idx < 0 || idx >= len(p.buckets) {
			return p, nil
		}
		p.selectedBucket = p.buckets[idx].Name
		p.mode = ModeKeyList
		p.keyErr = nil
		p.keys = nil
		p.keyTable.SetRows(nil)
		return p, p.loadKeysCmd()
	case "esc":
		// Handled by parent
	}
	return p, nil
}

func (p *KVStorePage) handleKeyListKeys(key string) (*KVStorePage, tea.Cmd) {
	switch key {
	case "j", "down":
		p.keyTable.MoveDown()
	case "k", "up":
		p.keyTable.MoveUp()
	case "g":
		p.keyTable.GoTop()
	case "G":
		p.keyTable.GoBottom()
	case "ctrl+d":
		p.keyTable.MovePageDown()
	case "ctrl+u":
		p.keyTable.MovePageUp()
	case "v":
		p.showKeyOverlay()
	case "r":
		return p, p.loadKeysCmd()
	case "enter":
		idx := p.keyTable.SelectedIndex()
		if idx < 0 || idx >= len(p.keys) {
			return p, nil
		}
		p.selectedKey = p.keys[idx]
		p.mode = ModeValueView
		p.valueErr = nil
		p.entry = nil
		p.deleteErr = nil
		p.deleteConfirm = false
		return p, p.loadValueCmd()
	case "d":
		idx := p.keyTable.SelectedIndex()
		if idx < 0 || idx >= len(p.keys) {
			return p, nil
		}
		p.selectedKey = p.keys[idx]
		p.deleteConfirm = true
		p.deleteErr = nil
	case "esc":
		p.mode = ModeBucketList
		p.keys = nil
		p.keyErr = nil
		p.keyTable.SetRows(nil)
	}
	return p, nil
}

func (p *KVStorePage) handleValueViewKeys(key string) (*KVStorePage, tea.Cmd) {
	switch key {
	case "r":
		return p, p.loadValueCmd()
	case "d":
		p.deleteConfirm = true
		p.deleteErr = nil
	case "esc":
		p.mode = ModeKeyList
		p.entry = nil
		p.valueErr = nil
		p.deleteErr = nil
		p.deleteConfirm = false
	}
	return p, nil
}

// SetSize

const kvToolbarRows = 2 // toolbar row + blank line

func (p *KVStorePage) SetSize(width, height int) {
	p.width = width
	p.height = height
	tableH := height - kvToolbarRows
	if tableH < 3 {
		tableH = 3
	}
	p.bucketTable.SetSize(width, tableH)
	p.keyTable.SetSize(width, tableH)
}

func (p *KVStorePage) SetToolbarOrigin(x, y int) {
	p.bucketToolbar.SetTopLeft(x, y)
	p.keyToolbar.SetTopLeft(x, y)
}

// FocusItems returns the focusable widgets for the current mode. Value-view
// and delete-confirm modes return nil (no interactive chrome — those modes
// use their own key handlers).
func (p *KVStorePage) FocusItems(activateRow func() tea.Cmd) []focus.Item {
	switch p.mode {
	case ModeBucketList:
		return []focus.Item{
			NewToolbarFocusItem(p.bucketToolbar, func(id string) tea.Cmd {
				return toolbarKeyCmd(id, kvActionKey)
			}),
			NewTableListFocusItem(p.bucketTable, activateRow),
		}
	case ModeKeyList:
		return []focus.Item{
			NewToolbarFocusItem(p.keyToolbar, func(id string) tea.Cmd {
				return toolbarKeyCmd(id, kvActionKey)
			}),
			NewTableListFocusItem(p.keyTable, activateRow),
		}
	}
	return nil
}

// FocusRouted disables focus routing when a modal (overlay / delete confirm)
// is up, or when we're in the value-view mode which has no toolbar/table.
func (p *KVStorePage) FocusRouted() bool {
	if p.deleteConfirm {
		return false
	}
	switch p.mode {
	case ModeBucketList:
		return !p.bucketOverlay.Visible()
	case ModeKeyList:
		return !p.keyOverlay.Visible()
	}
	return false
}

// ActivateSelectedBucket mirrors "Enter on a bucket row" — captures the
// selected bucket name, flips to key-list mode, and returns the load cmd.
// Called by the focus adapter's Enter callback.
func (p *KVStorePage) ActivateSelectedBucket() tea.Cmd {
	idx := p.bucketTable.SelectedIndex()
	if idx < 0 || idx >= len(p.buckets) {
		return nil
	}
	p.selectedBucket = p.buckets[idx].Name
	p.mode = ModeKeyList
	p.keyErr = nil
	p.keys = nil
	p.keyTable.SetRows(nil)
	return p.loadKeysCmd()
}

// ActivateSelectedKey mirrors "Enter on a key row" — captures the selected
// key name, flips to value-view mode, and returns the load cmd.
func (p *KVStorePage) ActivateSelectedKey() tea.Cmd {
	idx := p.keyTable.SelectedIndex()
	if idx < 0 || idx >= len(p.keys) {
		return nil
	}
	p.selectedKey = p.keys[idx]
	p.mode = ModeValueView
	p.valueErr = nil
	p.entry = nil
	p.deleteErr = nil
	p.deleteConfirm = false
	return p.loadValueCmd()
}

// View

func (p *KVStorePage) View() string {
	// Delete confirmation overlay
	if p.deleteConfirm {
		return p.renderDeleteConfirm()
	}

	switch p.mode {
	case ModeBucketList:
		return p.renderBucketList()
	case ModeKeyList:
		return p.renderKeyList()
	case ModeValueView:
		return p.renderValueView()
	}
	return ""
}

func (p *KVStorePage) renderBucketList() string {
	if p.bucketLoading {
		return "  Loading KV buckets..."
	}
	if p.bucketErr != nil {
		return lipgloss.NewStyle().Foreground(ui.Error).Render("  Error: " + p.bucketErr.Error())
	}
	var body string
	if len(p.buckets) == 0 {
		body = lipgloss.NewStyle().
			Foreground(ui.TextFaint).
			Italic(true).
			Render("  No KV buckets found")
	} else {
		body = p.bucketTable.View()
	}
	bg := p.bucketToolbar.View() + "\n\n" + body
	return p.bucketOverlay.PlaceOn(bg, p.width, p.height)
}

func (p *KVStorePage) renderKeyList() string {
	if p.keyLoading {
		return "  Loading keys..."
	}
	if p.keyErr != nil {
		return lipgloss.NewStyle().Foreground(ui.Error).Render("  Error: " + p.keyErr.Error())
	}
	var body string
	if len(p.keys) == 0 {
		body = lipgloss.NewStyle().
			Foreground(ui.TextFaint).
			Italic(true).
			Render("  No keys in bucket " + p.selectedBucket)
	} else {
		body = p.keyTable.View()
	}
	bg := p.keyToolbar.View() + "\n\n" + body
	return p.keyOverlay.PlaceOn(bg, p.width, p.height)
}

func (p *KVStorePage) showBucketOverlay() {
	idx := p.bucketTable.SelectedIndex()
	if idx < 0 || idx >= len(p.buckets) {
		return
	}
	b := p.buckets[idx]
	rows := []components.DetailRow{
		{Label: "Name", Value: b.Name},
		{Label: "Description", Value: emptyDash(b.Description)},
		{Label: "Values", Value: utils.FormatNumber(uint64(b.Values))},
		{Label: "History", Value: utils.FormatNumber(uint64(b.History))},
		{Label: "TTL", Value: formatMaxAge(b.TTL)},
		{Label: "Replicas", Value: utils.FormatNumber(uint64(b.Replicas))},
		{Label: "Stream", Value: b.StreamName},
		{Label: "Bytes", Value: utils.FormatBytes(b.Bytes)},
		{Label: "Created", Value: utils.FormatTime(b.Created)},
		{Label: "Age", Value: utils.FormatAge(b.Created)},
	}
	p.bucketOverlay.Show("Bucket: "+b.Name, rows)
}

func (p *KVStorePage) showKeyOverlay() {
	idx := p.keyTable.SelectedIndex()
	if idx < 0 || idx >= len(p.keys) {
		return
	}
	k := p.keys[idx]
	rows := []components.DetailRow{
		{Label: "Bucket", Value: p.selectedBucket},
		{Label: "Key", Value: k},
	}
	p.keyOverlay.Show("Key: "+k, rows)
}

func (p *KVStorePage) renderValueView() string {
	if p.valueLoading {
		return "  Loading value..."
	}
	if p.valueErr != nil {
		return lipgloss.NewStyle().Foreground(ui.Error).Render("  Error: " + p.valueErr.Error()) +
			"\n\n  Press r to retry or Esc to go back"
	}
	if p.entry == nil {
		return "  No entry data"
	}

	panelWidth := p.width - 4
	if panelWidth < 40 {
		panelWidth = 40
	}

	title := fmt.Sprintf("KV: %s/%s", p.selectedBucket, p.entry.Key)

	topLeft := lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u250c\u2500 ")
	topTitle := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true).Render(title)
	topRight := lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(" " + strings.Repeat("\u2500", panelWidth-len(title)-6) + "\u2510")

	var lines []string
	lines = append(lines, "  "+topLeft+topTitle+topRight)

	labelStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Width(12)
	valueStyle := lipgloss.NewStyle().Foreground(ui.TextColor)

	row := func(label, value string) string {
		l := labelStyle.Render("  \u2502  "+label)
		v := valueStyle.Render(value)
		return l + v
	}

	lines = append(lines, row("Bucket:", p.selectedBucket))
	lines = append(lines, row("Key:", p.entry.Key))
	lines = append(lines, row("Revision:", fmt.Sprintf("%d", p.entry.Revision)))
	lines = append(lines, row("Created:", utils.FormatAge(p.entry.Created)))

	midLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(
		"\u251c"+strings.Repeat("\u2500", panelWidth-2)+"\u2524",
	)
	lines = append(lines, midLine)

	// Word-wrap the value content
	wrappedValue := p.wrapText(p.entry.Value, panelWidth-4)
	for _, line := range strings.Split(wrappedValue, "\n") {
		contentLine := valueStyle.Render(line)
		padLen := panelWidth - 4 - lipgloss.Width(contentLine)
		if padLen < 0 {
			padLen = 0
		}
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2502")+"  "+contentLine+strings.Repeat(" ", padLen)+" "+lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render("\u2502"))
	}

	bottomLine := "  " + lipgloss.NewStyle().Foreground(ui.BrandSecondary).Render(
		"\u2514"+strings.Repeat("\u2500", panelWidth-2)+"\u2518",
	)
	lines = append(lines, bottomLine)

	// Hint line
	hintStyle := lipgloss.NewStyle().Foreground(ui.TextFaint)
	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("  d: Delete  r: Refresh  Esc: Back"))

	return strings.Join(lines, "\n")
}

func (p *KVStorePage) renderDeleteConfirm() string {
	keyName := p.selectedKey
	if p.mode == ModeValueView && p.entry != nil {
		keyName = p.entry.Key
	}

	panelWidth := p.width - 4
	if panelWidth < 40 {
		panelWidth = 40
	}

	var lines []string

	warningStyle := lipgloss.NewStyle().Foreground(ui.Warning).Bold(true)
	questionStyle := lipgloss.NewStyle().Foreground(ui.TextColor)
	hintStyle := lipgloss.NewStyle().Foreground(ui.TextFaint)
	borderStyle := lipgloss.NewStyle().Foreground(ui.Error)

	lines = append(lines, "  "+borderStyle.Render("\u250c"+strings.Repeat("\u2500", panelWidth-2)+"\u2510"))
	lines = append(lines, "  "+borderStyle.Render("\u2502")+"  "+warningStyle.Render("Delete Key?")+strings.Repeat(" ", panelWidth-16)+" "+borderStyle.Render("\u2502"))
	lines = append(lines, "  "+borderStyle.Render("\u2502")+strings.Repeat(" ", panelWidth-2)+borderStyle.Render("\u2502"))
	keyLabel := questionStyle.Render("  Are you sure you want to delete key: ")
	keyVal := lipgloss.NewStyle().Foreground(ui.Error).Bold(true).Render(keyName)
	contentLine := "  " + borderStyle.Render("\u2502") + keyLabel + keyVal
	padLen := panelWidth - 2 - lipgloss.Width(keyLabel) - lipgloss.Width(keyVal)
	if padLen < 1 {
		padLen = 1
	}
	contentLine += strings.Repeat(" ", padLen) + borderStyle.Render("\u2502")
	lines = append(lines, contentLine)
	lines = append(lines, "  "+borderStyle.Render("\u2502")+strings.Repeat(" ", panelWidth-2)+borderStyle.Render("\u2502"))

	confirmLine := "  " + borderStyle.Render("\u2502") + "  " + hintStyle.Render("Enter: Confirm   Esc: Cancel")
	padLen = panelWidth - 2 - lipgloss.Width(hintStyle.Render("Enter: Confirm   Esc: Cancel"))
	if padLen < 1 {
		padLen = 1
	}
	confirmLine += strings.Repeat(" ", padLen) + borderStyle.Render("\u2502")
	lines = append(lines, confirmLine)

	lines = append(lines, "  "+borderStyle.Render("\u2514"+strings.Repeat("\u2500", panelWidth-2)+"\u2518"))

	if p.deleteErr != nil {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.Error).Render("  Delete failed: "+p.deleteErr.Error()))
	}

	return strings.Join(lines, "\n")
}

// HelpText

func (p *KVStorePage) HelpText() string {
	if p.deleteConfirm {
		return "Enter: Confirm Delete  Esc: Cancel"
	}
	switch p.mode {
	case ModeBucketList:
		if p.bucketOverlay.Visible() {
			return "Esc: Close"
		}
		return "j/k: Navigate  g/G: Top/Bottom  n: New Bucket  d: Delete Bucket  r: Refresh  Enter: View Keys  v: Peek  Esc: Back"
	case ModeKeyList:
		if p.keyOverlay.Visible() {
			return "Esc: Close"
		}
		return "j/k: Navigate  g/G: Top/Bottom  n: New Key  d: Delete  r: Refresh  Enter: View  v: Peek  Esc: Back"
	case ModeValueView:
		return "e: Edit  d: Delete  r: Refresh  Esc: Back"
	}
	return ""
}

// StatusText

func (p *KVStorePage) StatusText() string {
	if p.deleteConfirm {
		return "Confirm Delete: " + p.selectedKey
	}
	switch p.mode {
	case ModeBucketList:
		if p.bucketLoading {
			return "Loading..."
		}
		return "KV Buckets: " + p.bucketTable.PositionText()
	case ModeKeyList:
		if p.keyLoading {
			return "Loading..."
		}
		return p.selectedBucket + " - Keys: " + p.keyTable.PositionText()
	case ModeValueView:
		if p.valueLoading {
			return "Loading..."
		}
		return p.selectedBucket + "/" + p.selectedKey
	}
	return ""
}

// Data conversion

func (p *KVStorePage) bucketsToRows(buckets []*natsclient.BucketInfo) [][]string {
	rows := make([][]string, len(buckets))
	for i, b := range buckets {
		rows[i] = []string{
			" " + b.Name,
			" " + utils.FormatNumber(uint64(b.Values)),
			" " + utils.FormatNumber(uint64(b.History)),
			" " + utils.FormatDuration(b.TTL),
			" " + utils.FormatBytes(b.Bytes),
			" " + utils.FormatAge(b.Created),
		}
	}
	return rows
}

func (p *KVStorePage) keysToRows(keys []string) [][]string {
	rows := make([][]string, len(keys))
	for i, k := range keys {
		rows[i] = []string{
			" " + k,
			"",
			"",
		}
	}
	return rows
}

// wrapText wraps text to fit within the given width
func (p *KVStorePage) wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var result strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			result.WriteByte('\n')
		}
		wrapped := p.wrapLine(line, width)
		result.WriteString(wrapped)
	}
	return result.String()
}

func (p *KVStorePage) wrapLine(line string, width int) string {
	if len(line) == 0 {
		return ""
	}
	if width <= 0 {
		return line
	}
	var result strings.Builder
	for len(line) > width {
		// Find the last space within width
		breakAt := width
		spaceIdx := strings.LastIndex(line[:width], " ")
		if spaceIdx > 0 {
			breakAt = spaceIdx + 1
			result.WriteString(line[:spaceIdx])
		} else {
			result.WriteString(line[:width])
		}
		result.WriteByte('\n')
		line = line[breakAt:]
	}
	result.WriteString(line)
	return result.String()
}

// Navigation helpers

func (p *KVStorePage) Mode() KVStoreMode {
	return p.mode
}

func (p *KVStorePage) SelectedBucket() string {
	return p.selectedBucket
}

// SelectedBucketInfo returns the currently highlighted bucket row, or nil
// when the table is empty. Used by app.go to snapshot the target of the
// delete-bucket confirm dialog at the moment it fires.
func (p *KVStorePage) SelectedBucketInfo() *natsclient.BucketInfo {
	idx := p.bucketTable.SelectedIndex()
	if idx < 0 || idx >= len(p.buckets) {
		return nil
	}
	return p.buckets[idx]
}

// ReloadBuckets returns a fresh load cmd for the bucket list, used by app.go
// to refresh after a bucket create/delete round-trip.
func (p *KVStorePage) ReloadBuckets() tea.Cmd {
	return p.loadBucketsCmd()
}

// ReloadKeys returns a fresh load cmd for the current bucket's key list —
// used by app.go to refresh after a create/edit-key round-trip.
func (p *KVStorePage) ReloadKeys() tea.Cmd {
	return p.loadKeysCmd()
}

// SelectedKeyName returns the currently highlighted key name in the key
// list, or "" when the table is empty.
func (p *KVStorePage) SelectedKeyName() string {
	idx := p.keyTable.SelectedIndex()
	if idx < 0 || idx >= len(p.keys) {
		return ""
	}
	return p.keys[idx]
}

// LoadedEntry returns the KV entry currently loaded in ModeValueView, or
// nil if none is loaded. Used by app.go to seed the edit form.
func (p *KVStorePage) LoadedEntry() *natsclient.KVEntry {
	return p.entry
}
