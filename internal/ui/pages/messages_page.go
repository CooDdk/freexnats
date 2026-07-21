package pages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
	"github.com/CooDdk/freexnats/pkg/utils"
)

const (
	ModeMessageView   = 0
	ModePublish       = 1
	ModeTail          = 2
	ModeMessageDetail = 3
)

type MessagesPage struct {
	client *natsclient.Client
	mode   int
	width  int
	height int

	// Stream population — kept as the source of truth for the selector's
	// dropdown and for resolving a name back to *StreamInfo on switch.
	streams []*natsclient.StreamInfo
	loading bool
	err     error

	// Persistent top-of-page selector. Clicking the pill (or Enter when
	// keyboard-focused) opens an inline dropdown listing all streams;
	// selection switches context in place without leaving the tab.
	selector                 *components.StreamSelector
	selectorKeyboardFocused  bool

	// Toolbars per mode. historyToolbar drives ModeMessageView (Publish,
	// Delete, Refresh). tailToolbar drives ModeTail (Publish, Refresh).
	historyToolbar *components.Toolbar
	tailToolbar    *components.Toolbar
	canWrite       bool

	// Message view
	selectedStream *natsclient.StreamInfo
	currentMsg     *natsclient.StoredMsg
	currentSeq     uint64
	msgLoading     bool
	msgErr         error
	payloadScroll  int

	// Publish sub-mode
	publish       *PublishForm
	publishReturn int // mode to restore on Esc
	// Origin cached from SetToolbarOrigin so we can forward it to the publish
	// form the moment it opens (SetToolbarOrigin fires on layout, but openPublish
	// creates the form lazily on user action — no origin has been set yet).
	publishOriginX int
	publishOriginY int

	// Tail sub-mode
	tail       *TailState
	tailReturn int

	// Page-level History/Live switcher — clickable at the top of the page in
	// list / message-view / tail modes. Coordinates recorded during View()
	// so absolute-Y hit testing survives outer lipgloss composition.
	segmentTopX, segmentTopY       int
	segmentHistX, segmentHistEndX  int
	segmentLiveX, segmentLiveEndX  int
	// segmentKeyboardFocused is true when the focus manager has parked
	// keyboard focus on the History/Live segment control. renderSegment
	// underlines the active pill in that case so the user can tell keyboard
	// focus is on the segment (distinct from the mode-indicator Primary bg).
	segmentKeyboardFocused bool

	// contentFocused signals that the message-content focus item owns
	// keyboard focus. renderMessagePanel switches its border to BrandPrimary
	// so the user can see ← / → will scrub prev/next-seq.
	contentFocused bool

	// Transient toast text (e.g. "Copied to clipboard") rendered as a small
	// pill in the top-right of the message panel. Cleared by toastClearMsg
	// after a short delay so the user gets feedback without the info
	// sticking around.
	toast string

	// Detail-mode scroll — separate from payloadScroll so entering /
	// leaving the detail view doesn't fight with the inline panel's state.
	detailScroll int
}

type MsgStreamsLoadedMsg struct {
	Streams []*natsclient.StreamInfo
	Err     error
}

// toastClearMsg fires ~2s after a yank so the "Copied" pill fades from the
// message panel. Delivered via tea.Tick.
type toastClearMsg struct{}

func clearToastAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg { return toastClearMsg{} })
}

type MsgLoadedMsg struct {
	Msg *natsclient.StoredMsg
	Err error
}

// MessageDeletedRefreshMsg carries the result of RefreshAfterDelete: fresh
// stream info + the message we landed on after seq clamping. Err is the last
// non-nil load error encountered while walking the seq range — informational,
// not fatal (empty stream also produces a nil Info + nil Msg).
type MessageDeletedRefreshMsg struct {
	Info *natsclient.StreamInfo
	Msg  *natsclient.StoredMsg
	Seq  uint64
	Err  error
}

func NewMessagesPage(client *natsclient.Client) *MessagesPage {
	p := &MessagesPage{
		client:   client,
		mode:     ModeMessageView,
		selector: components.NewStreamSelector(),
	}
	p.historyToolbar = components.NewToolbar("messages-history-toolbar", p.buildHistoryActions())
	p.tailToolbar = components.NewToolbar("messages-tail-toolbar", p.buildTailActions())
	return p
}

func (p *MessagesPage) SetConnected(ok bool) {
	p.canWrite = ok
	p.historyToolbar.SetActions(p.buildHistoryActions())
	p.tailToolbar.SetActions(p.buildTailActions())
}

func (p *MessagesPage) buildTailActions() []components.ToolbarAction {
	return []components.ToolbarAction{
		{ID: "publish", Label: "Publish Message", Icon: "+", Primary: true, Disabled: !p.canWrite},
		{ID: "refresh", Label: "Refresh"},
	}
}

func (p *MessagesPage) buildHistoryActions() []components.ToolbarAction {
	return []components.ToolbarAction{
		{ID: "publish", Label: "Publish Message", Icon: "+", Primary: true, Disabled: !p.canWrite},
		{ID: "delete", Label: "Delete", Disabled: !p.canWrite},
		{ID: "refresh", Label: "Refresh"},
	}
}

// messagesActionKey maps toolbar IDs to the keyboard shortcut equivalents so
// clicks flow through the same code paths as keypresses.
func messagesActionKey(id string) string {
	switch id {
	case "publish":
		return "P"
	case "delete":
		return "d"
	case "refresh":
		return "r"
	}
	return ""
}

func (p *MessagesPage) Init() tea.Cmd {
	p.loading = true
	p.err = nil
	return p.loadStreamsCmd()
}

func (p *MessagesPage) loadStreamsCmd() tea.Cmd {
	return func() tea.Msg {
		streams, err := p.client.GetAllStreamInfos()
		return MsgStreamsLoadedMsg{Streams: streams, Err: err}
	}
}

func (p *MessagesPage) loadMessageCmd(stream string, seq uint64) tea.Cmd {
	return func() tea.Msg {
		msg, err := p.client.GetStreamMessage(stream, seq)
		return MsgLoadedMsg{Msg: msg, Err: err}
	}
}

func (p *MessagesPage) Update(msg tea.Msg) (*MessagesPage, tea.Cmd) {
	switch msg := msg.(type) {
	case MsgStreamsLoadedMsg:
		p.loading = false
		if msg.Err != nil {
			p.err = msg.Err
			return p, nil
		}
		p.streams = msg.Streams
		names := make([]string, len(msg.Streams))
		for i, s := range msg.Streams {
			names[i] = s.Name
		}
		p.selector.SetStreams(names)
		// Auto-select the first stream on first population so the user lands
		// on real data. Also handles the "current selection disappeared"
		// case (stream deleted elsewhere) by falling back to the first.
		if p.selectedStream == nil && len(msg.Streams) > 0 {
			return p, p.switchStream(msg.Streams[0].Name)
		}
		if p.selectedStream != nil {
			found := false
			for _, s := range msg.Streams {
				if s.Name == p.selectedStream.Name {
					p.selectedStream = s
					found = true
					break
				}
			}
			if !found && len(msg.Streams) > 0 {
				return p, p.switchStream(msg.Streams[0].Name)
			}
			if !found {
				p.selectedStream = nil
				p.currentMsg = nil
			}
		}

	case MsgLoadedMsg:
		p.msgLoading = false
		if msg.Err != nil {
			p.msgErr = msg.Err
			return p, nil
		}
		p.currentMsg = msg.Msg

	case MessageDeletedRefreshMsg:
		// Stream metadata may have shifted (LastSeq, Messages count) — refresh.
		if msg.Info != nil {
			p.selectedStream = msg.Info
		}
		// Empty stream after delete → stay in Message View with an empty
		// panel; renderMessagePanel handles currentMsg == nil.
		if msg.Info == nil || msg.Info.Messages == 0 {
			p.currentMsg = nil
			p.msgErr = nil
			p.payloadScroll = 0
			return p, p.loadStreamsCmd()
		}
		p.currentSeq = msg.Seq
		p.currentMsg = msg.Msg
		p.msgLoading = false
		if msg.Msg == nil {
			p.msgErr = msg.Err
		} else {
			p.msgErr = nil
		}
		p.payloadScroll = 0
		return p, nil

	case PublishResultMsg:
		if p.publish != nil {
			p.publish.HandleResult(msg)
		}
		return p, nil

	case TailPollMsg:
		if p.mode != ModeTail || p.tail == nil {
			return p, nil
		}
		return p, p.tail.OnPoll()

	case toastClearMsg:
		p.toast = ""
		return p, nil

	case tea.MouseMsg:
		// Selector owns clicks when open; also route pill clicks (closed state)
		// into it so opening the dropdown is uniform across modes.
		if chosen, closed, handled := p.selector.HandleMouse(msg); handled {
			_ = closed
			if chosen != "" {
				return p, p.switchStream(chosen)
			}
			return p, nil
		}
		if p.mode == ModeMessageView {
			if cmd, hit := p.handleSegmentMouse(msg); hit {
				return p, cmd
			}
			if id, hit := p.historyToolbar.HandleMouse(msg); hit {
				return p, toolbarKeyCmd(id, messagesActionKey)
			}
			// Wheel over the page scrolls the payload. Cheap discovery for
			// long JSON payloads that overflow the panel — user doesn't need
			// to know j/k. Same 1-line step as j/k for parity.
			if msg.Button == tea.MouseButtonWheelUp {
				if p.payloadScroll > 0 {
					p.payloadScroll--
				}
				return p, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				if p.payloadScroll < p.maxPayloadScroll() {
					p.payloadScroll++
				}
				return p, nil
			}
		} else if p.mode == ModeMessageDetail {
			// Wheel scrolls the detail payload viewer, mirroring j/k.
			if msg.Button == tea.MouseButtonWheelUp {
				if p.detailScroll > 0 {
					p.detailScroll--
				}
				return p, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				if p.detailScroll < p.maxDetailScroll() {
					p.detailScroll++
				}
				return p, nil
			}
		} else if p.mode == ModePublish {
			if p.publish != nil {
				if cmd, handled := p.publish.HandleMouse(msg, p.client); handled {
					return p, cmd
				}
			}
		} else if p.mode == ModeTail {
			if cmd, hit := p.handleSegmentMouse(msg); hit {
				return p, cmd
			}
			// The top row in Tail mode uses tailToolbar (Publish · Refresh).
			// Route toolbar clicks BEFORE falling through to the tail state's
			// own hit-testing, otherwise those buttons look interactive but
			// do nothing.
			if id, hit := p.tailToolbar.HandleMouse(msg); hit {
				return p, toolbarKeyCmd(id, messagesActionKey)
			}
			if p.tail != nil {
				cmd, handled, back := p.tail.HandleMouse(msg)
				if back {
					return p.exitTail(), nil
				}
				if handled {
					return p, cmd
				}
			}
		}

	case tea.KeyMsg:
		// While the selector's dropdown is open it owns the keyboard.
		if p.selector.IsOpen() {
			if chosen, _, handled := p.selector.HandleKey(msg); handled {
				if chosen != "" {
					return p, p.switchStream(chosen)
				}
				return p, nil
			}
		}
		switch p.mode {
		case ModeMessageView:
			return p.updateMessageView(msg)
		case ModePublish:
			return p.updatePublish(msg)
		case ModeTail:
			return p.updateTail(msg)
		case ModeMessageDetail:
			return p.updateMessageDetail(msg)
		}
	}

	return p, nil
}

// switchStream is the single entry point for changing the active stream. It
// updates selector state, aligns Publish/Tail sub-modes with the new target
// (Tail unsubscribes + resubscribes for a clean slate), and dispatches a
// message load in Message View.
func (p *MessagesPage) switchStream(name string) tea.Cmd {
	var target *natsclient.StreamInfo
	for _, s := range p.streams {
		if s.Name == name {
			target = s
			break
		}
	}
	if target == nil {
		return nil
	}
	p.selector.SetCurrent(name)

	switch p.mode {
	case ModeTail:
		// Tear down existing tail and re-open on the new stream. exitTail
		// restores p.mode to tailReturn, so remember and restore ModeTail
		// after openTail() sets it again.
		if p.tail != nil {
			p.tail.Stop()
			p.tail = nil
		}
		p.selectedStream = target
		p.currentSeq = target.LastSeq
		p.currentMsg = nil
		p.payloadScroll = 0
		if !p.client.IsConnected() {
			return nil
		}
		p.tail = newTailState(target)
		p.tail.SetSize(p.width, p.height-messagesToolbarRows)
		p.tail.SetOrigin(p.publishOriginX, p.publishOriginY+messagesToolbarRows)
		return p.tail.Start(p.client)
	case ModePublish:
		// Keep the user on the form; just update the target. Publish reads
		// selectedStream at submit time.
		p.selectedStream = target
		return nil
	default: // ModeMessageView
		p.selectedStream = target
		p.currentSeq = target.LastSeq
		p.currentMsg = nil
		p.msgErr = nil
		p.payloadScroll = 0
		if target.Messages == 0 || target.LastSeq == 0 {
			return nil
		}
		return p.loadCurrentMsgCmd()
	}
}

func (p *MessagesPage) updateMessageView(msg tea.KeyMsg) (*MessagesPage, tea.Cmd) {
	switch msg.String() {
	case "l", "right":
		if p.selectedStream != nil && p.currentSeq < p.selectedStream.LastSeq {
			p.currentSeq++
			return p, p.loadCurrentMsgCmd()
		}
	case "h", "left":
		if p.selectedStream != nil && p.currentSeq > p.selectedStream.FirstSeq {
			p.currentSeq--
			return p, p.loadCurrentMsgCmd()
		}
	case "j", "down":
		if p.payloadScroll < p.maxPayloadScroll() {
			p.payloadScroll++
		}
	case "k", "up":
		if p.payloadScroll > 0 {
			p.payloadScroll--
		}
	case "ctrl+d", "pgdown":
		p.payloadScroll += p.payloadVisibleRows()
		if max := p.maxPayloadScroll(); p.payloadScroll > max {
			p.payloadScroll = max
		}
	case "ctrl+u", "pgup":
		p.payloadScroll -= p.payloadVisibleRows()
		if p.payloadScroll < 0 {
			p.payloadScroll = 0
		}
	case "g":
		p.payloadScroll = 0
	case "G":
		p.payloadScroll = p.maxPayloadScroll()
	case "r":
		if p.selectedStream != nil {
			return p, p.loadCurrentMsgCmd()
		}
	case "P", "shift+p", "p":
		return p.openPublish()
	case "t":
		return p.openTail()
	case "y":
		return p, p.yankPayload()
	case "v":
		return p.openDetail()
	}

	return p, nil
}

func (p *MessagesPage) loadCurrentMsgCmd() tea.Cmd {
	p.msgLoading = true
	p.msgErr = nil
	p.currentMsg = nil
	p.payloadScroll = 0
	return p.loadMessageCmd(p.selectedStream.Name, p.currentSeq)
}

const messagesToolbarRows = 2 // toolbar row (with inline segment on left) + blank line

func (p *MessagesPage) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *MessagesPage) SetToolbarOrigin(x, y int) {
	// Segment sits INLINE on the same row as the toolbar (segment on the left,
	// action buttons on the right) so the page's top chrome height matches
	// every other tab. The toolbar's absolute X depends on segment width and
	// is computed lazily inside renderTopBar; defer SetTopLeft until then.
	p.segmentTopX = x
	p.segmentTopY = y
	if p.publish != nil {
		p.publish.SetOrigin(x, y)
	}
	p.publishOriginX = x
	p.publishOriginY = y
}

// FocusItems returns the arrow-navigable widgets for the current mode.
// ModeMessageView and ModeTail expose: Stream selector pill → History/Live
// segment → toolbar on the top row, plus a message-content item BELOW that
// captures ← / → for prev/next-seq navigation. ↓ from any top-row widget
// jumps straight to the content item (skipping same-row neighbours) via
// focus.Jump; ↑ from content returns to the segment. ModePublish wraps the
// PublishForm opaquely so ↑ escapes to Tabs from the Subject field.
//
// baseIdx is where this page's first item lands in the composite focus list
// (the parent app prepends its own items — Tabs — before this page's). The
// Jump targets we compute must be absolute to that composite list, so callers
// must pass len(preceding-items).
func (p *MessagesPage) FocusItems(baseIdx int) []focus.Item {
	switch p.mode {
	case ModeMessageView:
		sel := &streamSelectorFocusItem{page: p}
		seg := &messagesSegmentFocusItem{page: p}
		tb := NewToolbarFocusItem(p.historyToolbar, func(id string) tea.Cmd {
			return toolbarKeyCmd(id, messagesActionKey)
		})
		content := &messagesContentFocusItem{page: p}
		// Content is the 4th (local index 3) item; top-row items' ↓ jump
		// straight to it, skipping the toolbar on the same visual row.
		sel.downTarget = baseIdx + 3
		seg.downTarget = baseIdx + 3
		content.upTarget = baseIdx + 1 // return to segment on ↑
		return []focus.Item{sel, seg, tb, content}
	case ModePublish:
		if p.publish == nil {
			return nil
		}
		return []focus.Item{
			NewPublishFormFocusItem(p.publish, p.client),
		}
	case ModeTail:
		sel := &streamSelectorFocusItem{page: p}
		seg := &messagesSegmentFocusItem{page: p}
		tb := NewToolbarFocusItem(p.tailToolbar, func(id string) tea.Cmd {
			return toolbarKeyCmd(id, messagesActionKey)
		})
		// Tail has no page-level content focus item (the tail panel manages
		// its own scroll via j/k); ↓ from the top row falls through to the
		// toolbar via linear escape.
		return []focus.Item{sel, seg, tb}
	}
	return nil
}

// FocusRouted disables focus routing during modal overlays and in modes
// that don't participate in the focus manager. The selector dropdown owns
// keyboard focus while open — return false so the manager doesn't fight
// the picker. Tail mode joins the focus system so ← → can walk the top
// row, but the filter textinput takes over arrow keys while it's being
// edited.
func (p *MessagesPage) FocusRouted() bool {
	if p.selector != nil && p.selector.IsOpen() {
		return false
	}
	switch p.mode {
	case ModeMessageView:
		return true
	case ModeMessageDetail:
		// Full-page viewer owns keys itself (esc/v/j/k/y/h/l). Don't let
		// the focus manager fight with our shortcuts.
		return false
	case ModePublish:
		return p.publish != nil && !p.publish.Success()
	case ModeTail:
		if p.tail == nil {
			return false
		}
		return !p.tail.IsFilterFocused()
	}
	return false
}

// HasSelectedMessage reports whether the page is in message-view mode with a
// concrete stream + seq that can be targeted by app.go's Delete key handler.
// Requires currentMsg to be loaded so we don't offer "delete" on a placeholder.
func (p *MessagesPage) HasSelectedMessage() bool {
	return p.mode == ModeMessageView && p.selectedStream != nil && p.currentMsg != nil
}

// SelectedStreamAndSeq returns the target of a delete op — the current
// stream name + seq being viewed. Only meaningful when HasSelectedMessage
// returns true.
func (p *MessagesPage) SelectedStreamAndSeq() (string, uint64) {
	if p.selectedStream == nil {
		return "", 0
	}
	return p.selectedStream.Name, p.currentSeq
}

// RefreshAfterDelete rebuilds the message-view state after a successful
// DeleteStreamMessage: pull fresh stream info (Messages/FirstSeq/LastSeq may
// have shifted), then walk seq forward-then-backward from the deleted seq to
// find the nearest surviving message so the user's cursor lands somewhere
// meaningful. Emits MessageDeletedRefreshMsg for Update to consume.
func (p *MessagesPage) RefreshAfterDelete() tea.Cmd {
	if p.selectedStream == nil {
		return nil
	}
	stream := p.selectedStream.Name
	deletedSeq := p.currentSeq
	client := p.client
	return func() tea.Msg {
		info, err := client.GetStreamInfo(stream)
		if err != nil || info == nil {
			return MessageDeletedRefreshMsg{Err: err}
		}
		if info.Messages == 0 {
			return MessageDeletedRefreshMsg{Info: info}
		}
		// Try seqs walking outward from the deleted position: prefer the
		// next surviving message (deletedSeq+1..LastSeq), fall back to the
		// prior one (deletedSeq-1..FirstSeq).
		var (
			landed  *natsclient.StoredMsg
			landSeq uint64
			lastErr error
		)
		for s := deletedSeq; s <= info.LastSeq; s++ {
			m, e := client.GetStreamMessage(stream, s)
			if e == nil {
				landed = m
				landSeq = s
				break
			}
			lastErr = e
		}
		if landed == nil && deletedSeq > info.FirstSeq {
			for s := deletedSeq - 1; s >= info.FirstSeq; s-- {
				m, e := client.GetStreamMessage(stream, s)
				if e == nil {
					landed = m
					landSeq = s
					break
				}
				lastErr = e
				if s == 0 {
					break
				}
			}
		}
		return MessageDeletedRefreshMsg{Info: info, Msg: landed, Seq: landSeq, Err: lastErr}
	}
}

func (p *MessagesPage) View() string {
	switch p.mode {
	case ModeMessageView:
		return p.viewMessageView()
	case ModePublish:
		return p.viewPublish()
	case ModeTail:
		return p.viewTail()
	case ModeMessageDetail:
		return p.viewMessageDetail()
	}
	return ""
}

func (p *MessagesPage) openPublish() (*MessagesPage, tea.Cmd) {
	stream := p.publishStream()
	if stream == nil {
		return p, nil
	}
	defaultSubject := ""
	if len(stream.Subjects) > 0 {
		defaultSubject = firstConcreteSubject(stream.Subjects[0])
	}
	p.publish = NewPublishForm(stream.Name, defaultSubject)
	p.publish.SetWidth(p.width)
	p.publish.SetHeight(p.height)
	p.publish.SetOrigin(p.publishOriginX, p.publishOriginY)
	p.publishReturn = p.mode
	p.mode = ModePublish
	return p, nil
}

func (p *MessagesPage) publishStream() *natsclient.StreamInfo {
	return p.selectedStream
}

func (p *MessagesPage) updatePublish(msg tea.KeyMsg) (*MessagesPage, tea.Cmd) {
	if msg.String() == "esc" {
		p.mode = p.publishReturn
		p.publish = nil
		return p, nil
	}
	if p.publish == nil {
		return p, nil
	}
	p.publish.SetWidth(p.width)
	p.publish.SetHeight(p.height)
	cmd, _ := p.publish.Update(msg, p.client)
	return p, cmd
}

func (p *MessagesPage) viewPublish() string {
	if p.publish == nil {
		return ""
	}
	p.publish.SetWidth(p.width)
	p.publish.SetHeight(p.height)
	return p.publish.View()
}

// openTail enters live tail mode subscribing to the currently selected stream's
// subjects. No-op if no stream is selected or if the client isn't connected.
func (p *MessagesPage) openTail() (*MessagesPage, tea.Cmd) {
	stream := p.publishStream()
	if stream == nil {
		return p, nil
	}
	if !p.client.IsConnected() {
		return p, nil
	}
	p.tail = newTailState(stream)
	p.tail.SetSize(p.width, p.height-messagesToolbarRows)
	p.tail.SetOrigin(p.publishOriginX, p.publishOriginY+messagesToolbarRows)
	p.tailReturn = p.mode
	p.mode = ModeTail
	return p, p.tail.Start(p.client)
}

// exitTail tears down the subscription and restores the previous mode.
func (p *MessagesPage) exitTail() *MessagesPage {
	if p.tail != nil {
		p.tail.Stop()
		p.tail = nil
	}
	p.mode = p.tailReturn
	return p
}

func (p *MessagesPage) updateTail(msg tea.KeyMsg) (*MessagesPage, tea.Cmd) {
	if p.tail == nil {
		return p, nil
	}
	// While the filter textinput is focused it owns every key except our
	// two explicit exits: Enter commits, Esc cancels the edit (does NOT
	// leave Tail mode). This mirrors the Form → outer-page contract.
	if p.tail.IsFilterFocused() {
		switch msg.String() {
		case "enter":
			p.tail.CommitFilterEdit()
			return p, nil
		case "esc":
			p.tail.CancelFilterEdit()
			return p, nil
		}
		cmd := p.tail.UpdateFilterInput(msg)
		return p, cmd
	}
	switch msg.String() {
	case "esc":
		// If a filter is active but not focused, Esc clears it instead of
		// leaving Tail mode. Second Esc exits.
		if p.tail.filter != "" {
			p.tail.ClearFilter()
			return p, nil
		}
		return p.exitTail(), nil
	case "/":
		p.tail.StartFilterEdit()
		return p, nil
	case " ", "space":
		p.tail.TogglePause()
	case "c":
		p.tail.Clear()
	case "j", "down":
		p.tail.ScrollDown(1)
	case "k", "up":
		p.tail.ScrollUp(1)
	case "ctrl+d", "pgdown":
		p.tail.ScrollDown(10)
	case "ctrl+u", "pgup":
		p.tail.ScrollUp(10)
	case "G":
		p.tail.autoBot = true
		p.tail.scroll = 0
	// Top-row toolbar shortcuts. The tailToolbar (Publish Message · Refresh)
	// is rendered above the tail panel and toolbar clicks synthesize these
	// key events (see messagesActionKey), so Tail mode must handle them
	// the same as Message View — otherwise the buttons flash but do nothing.
	// "P" is the toolbar's Publish action; lowercase "p" toggles pause, kept
	// separate so the two shortcuts don't collide.
	case "P", "shift+p":
		return p.openPublish()
	case "p":
		p.tail.TogglePause()
	case "r":
		// Refresh reloads the stream list in the background (same as History
		// mode). No visible effect while the tail is on-screen, but the
		// underlying stream metadata is picked up so exiting to History
		// shows fresh data.
		return p, p.loadStreamsCmd()
	}
	return p, nil
}

func (p *MessagesPage) viewTail() string {
	if p.tail == nil {
		return ""
	}
	// Inline top bar (selector + segment + toolbar on one row) sits above the
	// tail panel so page-level navigation stays consistent with other Messages
	// modes.
	p.tail.SetSize(p.width, p.height-messagesToolbarRows)
	p.tail.SetOrigin(p.publishOriginX, p.publishOriginY+messagesToolbarRows)
	base := p.renderTopBar(p.tailToolbar) + "\n\n" + p.tail.View()
	return p.spliceSelectorDropdown(base)
}

// yankPayload copies the current message's raw payload to the system
// clipboard and flashes a "Copied to clipboard" toast on the message panel.
// No-op if there's nothing to copy.
func (p *MessagesPage) yankPayload() tea.Cmd {
	if p.currentMsg == nil {
		return nil
	}
	if err := clipboard.WriteAll(p.currentMsg.Data); err != nil {
		p.toast = "Clipboard error"
		return clearToastAfter(2 * time.Second)
	}
	p.toast = fmt.Sprintf("Copied %s to clipboard", utils.FormatBytes(uint64(len(p.currentMsg.Data))))
	return clearToastAfter(2 * time.Second)
}

// openDetail enters the full-page payload viewer. No-op without a loaded
// message. The panel keeps its own scroll offset so re-entering resets to
// the top rather than inheriting the inline panel's scroll position.
func (p *MessagesPage) openDetail() (*MessagesPage, tea.Cmd) {
	if p.currentMsg == nil {
		return p, nil
	}
	p.detailScroll = 0
	p.mode = ModeMessageDetail
	return p, nil
}

// updateMessageDetail handles keys while the full-page payload viewer is
// active. j/k/PgDn/PgUp/g/G scroll; y copies; h/l walk prev/next seq (same
// as the inline panel) and re-open the viewer on the new message; Esc /
// v close and return to the inline message view.
func (p *MessagesPage) updateMessageDetail(msg tea.KeyMsg) (*MessagesPage, tea.Cmd) {
	switch msg.String() {
	case "esc", "v", "q":
		p.mode = ModeMessageView
		return p, nil
	case "j", "down":
		if p.detailScroll < p.maxDetailScroll() {
			p.detailScroll++
		}
	case "k", "up":
		if p.detailScroll > 0 {
			p.detailScroll--
		}
	case "ctrl+d", "pgdown":
		p.detailScroll += p.detailVisibleRows()
		if m := p.maxDetailScroll(); p.detailScroll > m {
			p.detailScroll = m
		}
	case "ctrl+u", "pgup":
		p.detailScroll -= p.detailVisibleRows()
		if p.detailScroll < 0 {
			p.detailScroll = 0
		}
	case "g":
		p.detailScroll = 0
	case "G":
		p.detailScroll = p.maxDetailScroll()
	case "h", "left":
		if p.selectedStream != nil && p.currentSeq > p.selectedStream.FirstSeq {
			p.currentSeq--
			p.detailScroll = 0
			return p, p.loadCurrentMsgCmd()
		}
	case "l", "right":
		if p.selectedStream != nil && p.currentSeq < p.selectedStream.LastSeq {
			p.currentSeq++
			p.detailScroll = 0
			return p, p.loadCurrentMsgCmd()
		}
	case "y":
		return p, p.yankPayload()
	}
	return p, nil
}

// detailChrome = top header row + blank + panel borders (2). The full-page
// viewer has no top bar / stream-info panel — it's a focused reading view.
const detailChrome = 4

func (p *MessagesPage) detailVisibleRows() int {
	r := p.height - detailChrome
	if r < 3 {
		r = 3
	}
	return r
}

func (p *MessagesPage) detailPayloadLines() []string {
	if p.currentMsg == nil {
		return nil
	}
	innerW := p.width - 8
	if innerW < 20 {
		innerW = 20
	}
	lines, _ := formatPayload(p.currentMsg.Data, innerW)
	return lines
}

func (p *MessagesPage) maxDetailScroll() int {
	lines := p.detailPayloadLines()
	visible := p.detailVisibleRows() - detailMetaRows(p.currentMsg) - 1 // -1 for indicator when scrollable
	if visible < 1 {
		visible = 1
	}
	if len(lines) <= visible {
		return 0
	}
	return len(lines) - visible
}

// detailMetaRows counts the fixed rows above the payload in the detail view:
// title + blank + subject + timestamp + (optional headers block) + blank +
// "─ Payload" section header.
func detailMetaRows(msg *natsclient.StoredMsg) int {
	rows := 4 + 2 // title, blank, subject, timestamp, blank, "Payload"
	if msg != nil && len(msg.Headers) > 0 {
		rows += 2 + len(msg.Headers)
	}
	return rows
}

func (p *MessagesPage) viewMessageDetail() string {
	if p.currentMsg == nil {
		return ""
	}
	w := p.width - 4
	if w < 30 {
		w = 30
	}
	msg := p.currentMsg

	titleStyle := lipgloss.NewStyle().Foreground(ui.BrandSecondary).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(ui.TextFaint)
	sectionStyle := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Width(11)
	valueStyle := lipgloss.NewStyle().Foreground(ui.TextColor)

	lines, kind := formatPayload(msg.Data, w-4)
	kindLabel := map[string]string{"json": "JSON", "text": "Text", "empty": "Empty"}[kind]

	// Header line: seq + kind + size on the left, back-hint on the right.
	title := titleStyle.Render("Message #"+utils.FormatSeq(msg.Sequence)) +
		"  " + metaStyle.Render("("+kindLabel+", "+utils.FormatBytes(uint64(len(msg.Data)))+")")
	back := lipgloss.NewStyle().Foreground(ui.TextFaint).Render("Esc: Back")
	spacer := w - lipgloss.Width(title) - lipgloss.Width(back)
	if spacer < 1 {
		spacer = 1
	}
	header := "  " + title + strings.Repeat(" ", spacer) + back

	var body []string
	body = append(body, "")
	body = append(body, labelStyle.Render("Subject:")+"  "+valueStyle.Render(msg.Subject))
	body = append(body, labelStyle.Render("Timestamp:")+"  "+valueStyle.Render(msg.Timestamp))

	if len(msg.Headers) > 0 {
		body = append(body, "", sectionStyle.Render("─ Headers"))
		keys := make([]string, 0, len(msg.Headers))
		for k := range msg.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		hdrKeyStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Width(14)
		for _, k := range keys {
			v := strings.Join(msg.Headers[k], ", ")
			body = append(body, hdrKeyStyle.Render(k+":")+valueStyle.Render(v))
		}
	}

	body = append(body, "", sectionStyle.Render("─ Payload"))

	// Payload gets whatever rows are left inside the panel budget.
	panelBudget := p.height - detailChrome
	if panelBudget < 3 {
		panelBudget = 3
	}
	payloadBudget := panelBudget - len(body)
	needIndicator := len(lines) > payloadBudget
	if needIndicator {
		payloadBudget--
	}
	if payloadBudget < 1 {
		payloadBudget = 1
	}

	maxScroll := 0
	if len(lines) > payloadBudget {
		maxScroll = len(lines) - payloadBudget
	}
	if p.detailScroll > maxScroll {
		p.detailScroll = maxScroll
	}
	if p.detailScroll < 0 {
		p.detailScroll = 0
	}
	start := p.detailScroll
	end := start + payloadBudget
	if end > len(lines) {
		end = len(lines)
	}
	for i := start; i < end; i++ {
		body = append(body, valueStyle.Render(lines[i]))
	}
	for i := end - start; i < payloadBudget; i++ {
		body = append(body, "")
	}
	if needIndicator {
		indicator := fmt.Sprintf("  [%d-%d / %d lines]  j/k · PgUp/PgDn · g/G · y: Copy · Shift+drag: Select",
			start+1, end, len(lines))
		body = append(body, metaStyle.Render(indicator))
	} else {
		body = append(body, metaStyle.Render("  y: Copy · Shift+drag: Select"))
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.BrandPrimary).
		Padding(0, 2).
		Width(w).
		Render(strings.Join(body, "\n"))

	view := header + "\n" + panel
	if p.toast != "" {
		view = p.spliceToast(view)
	}
	return view
}

// spliceToast overlays a small green "toast" pill in the top-right of the
// view (both in Message View and Detail). Cheap feedback without stealing
// focus or shifting layout.
func (p *MessagesPage) spliceToast(view string) string {
	pill := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0b1220")).
		Background(ui.BrandPrimary).
		Bold(true).
		Padding(0, 2).
		Render("✓ " + p.toast)
	pillW := lipgloss.Width(pill)
	x := p.width - pillW - 4
	if x < 2 {
		x = 2
	}
	return components.PlaceOverlayAt(view, pill, x, 0)
}

// spliceSelectorDropdown overlays the selector's dropdown on top of an
// already-rendered page view. No-op when the selector is closed.
//
// Note on coord systems: pill mouse hit-testing uses ABSOLUTE screen coords
// (needed for tea.MouseMsg which arrives in absolute terms), but we're
// splicing inside MessagesPage.View() BEFORE app.go's PaddingLeft wraps the
// output. Within the un-padded view the pill is always at (0, 0), so the
// dropdown must be spliced at view-relative (0, 1) — one row under the top
// bar. Using the selector's absolute coords here would overshoot by exactly
// contentSidePad columns and push the dropdown off-screen right.
func (p *MessagesPage) spliceSelectorDropdown(view string) string {
	if p.selector == nil || !p.selector.IsOpen() {
		return view
	}
	dd, _, _, ok := p.selector.RenderDropdown()
	if !ok {
		return view
	}
	return components.PlaceOverlayAt(view, dd, 0, 1)
}

// renderTopBar draws the page-level Stream selector + History/Live Tail
// switcher inline with the passed-in toolbar (selector on the left, segment
// in the middle, action buttons on the right). Hit-test coordinates are
// recorded here and the toolbar's absolute X is set lazily based on selector
// + segment width.
func (p *MessagesPage) renderTopBar(toolbar *components.Toolbar) string {
	pill := p.selector.RenderPill()
	pillW := p.selector.PillWidth()
	p.selector.SetOrigin(p.segmentTopX, p.segmentTopY)

	liveActive := p.mode == ModeTail
	// Underline only the ACTIVE side when the segment has keyboard focus —
	// two underlines feel busy; one is a clear "focus is here" signal on
	// top of the Primary-bg mode indicator.
	history := renderSegment("History", !liveActive, p.segmentKeyboardFocused && !liveActive)
	live := renderSegment("● Live Tail", liveActive, p.segmentKeyboardFocused && liveActive)
	seg := history + live
	segWidth := lipgloss.Width(seg)

	const pillGap = 2
	segLeftX := p.segmentTopX + pillW + pillGap
	p.segmentHistX = segLeftX
	p.segmentHistEndX = p.segmentHistX + lipgloss.Width(history) - 1
	p.segmentLiveX = p.segmentHistEndX + 1
	p.segmentLiveEndX = p.segmentLiveX + lipgloss.Width(live) - 1

	const gap = 4 // visual breathing room between segment and toolbar buttons
	toolbar.SetTopLeft(segLeftX+segWidth+gap, p.segmentTopY)

	return pill + strings.Repeat(" ", pillGap) + seg + strings.Repeat(" ", gap) + toolbar.View()
}

// handleSegmentMouse routes clicks on the page-level History/Live switcher.
// Returns (cmd, hit): hit=true means the click landed on a segment and
// callers should not fall through to other widgets.
func (p *MessagesPage) handleSegmentMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false
	}
	// ±1 row tolerance for Y underreport, same convention as Tabs/Toolbar.
	if msg.Y != p.segmentTopY && msg.Y+1 != p.segmentTopY {
		return nil, false
	}
	if msg.X >= p.segmentHistX && msg.X <= p.segmentHistEndX {
		if p.mode == ModeTail {
			p.exitTail()
		}
		return nil, true
	}
	if msg.X >= p.segmentLiveX && msg.X <= p.segmentLiveEndX {
		if p.mode != ModeTail {
			_, cmd := p.openTail()
			return cmd, true
		}
		return nil, true
	}
	return nil, false
}

// firstConcreteSubject strips wildcards from a subject filter to produce a
// reasonable default. "orders.*" -> "orders.new", "orders.>" -> "orders.new".
// A concrete subject is returned as-is.
func firstConcreteSubject(subj string) string {
	if subj == "" {
		return ""
	}
	if !strings.ContainsAny(subj, "*>") {
		return subj
	}
	parts := strings.Split(subj, ".")
	for i, p := range parts {
		if p == "*" || p == ">" {
			parts[i] = "new"
		}
	}
	return strings.Join(parts, ".")
}

func (p *MessagesPage) viewMessageView() string {
	if p.loading {
		return "  Loading streams..."
	}
	if p.err != nil {
		return "  Error: " + p.err.Error()
	}

	// No streams at all: show the top bar (with an empty selector pill) plus
	// a centered "no streams" panel so the layout stays put.
	if p.selectedStream == nil {
		top := p.renderTopBar(p.historyToolbar)
		empty := lipgloss.NewStyle().
			Foreground(ui.TextFaint).
			Italic(true).
			Render("No streams on this server. Create one from the Streams tab.")
		body := lipgloss.NewStyle().
			Width(p.width - 4).
			Align(lipgloss.Center).
			Render(empty)
		return p.spliceSelectorDropdown(top + "\n\n" + body)
	}

	var sections []string
	sections = append(sections, p.renderTopBar(p.historyToolbar))
	sections = append(sections, "")
	sections = append(sections, p.renderStreamInfoPanel())
	sections = append(sections, "")
	sections = append(sections, p.renderMessagePanel())

	out := p.spliceSelectorDropdown(strings.Join(sections, "\n"))
	if p.toast != "" {
		out = p.spliceToast(out)
	}
	return out
}

func (p *MessagesPage) renderStreamInfoPanel() string {
	s := p.selectedStream
	w := p.width - 4
	if w < 20 {
		w = 20
	}

	// One-liner: the selector pill already advertises the current stream
	// name, so we skip re-displaying it and keep just the operational bits.
	subjectsStr := strings.Join(s.Subjects, ", ")
	if subjectsStr == "" {
		subjectsStr = "-"
	}
	subLabel := lipgloss.NewStyle().Foreground(ui.SubtleColor).Render("Subjects: ")
	subValue := lipgloss.NewStyle().Foreground(ui.TextColor).Render(subjectsStr)
	sep := lipgloss.NewStyle().Foreground(ui.TextFaint).Render(" \u00B7 ")
	msgs := lipgloss.NewStyle().Foreground(ui.TextColor).Render(utils.FormatNumber(s.Messages) + " msgs")
	rng := lipgloss.NewStyle().Foreground(ui.TextColor).
		Render(fmt.Sprintf("seq %s\u2013%s", utils.FormatSeq(s.FirstSeq), utils.FormatSeq(s.LastSeq)))

	line := "  " + subLabel + subValue + sep + msgs + sep + rng
	// Clip to width so long subject lists can't blow up the layout.
	if lipgloss.Width(line) > w {
		line = ansiClip(line, w)
	}
	return line
}

// ansiClip truncates s so its visible width is \u2264 maxW, appending an ellipsis
// when clipped. Uses lipgloss.Width for measurement so ANSI escapes don't count.
func ansiClip(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}
	// runewidth-aware rune walk; strip ANSI is overkill for our uses since
	// the styles here don't include cursor moves.
	out := make([]rune, 0, len(s))
	w := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			out = append(out, r)
			continue
		}
		if inEsc {
			out = append(out, r)
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		rw := runewidth.RuneWidth(r)
		if w+rw > maxW-1 {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out) + "\u2026"
}

// Fixed chrome around the message panel content:
//   top bar row (selector + segment + toolbar inline) + blank: 2
//   streamInfo one-liner + blank: 2
//   message panel borders (top+bottom): 2
const messagePanelChrome = 6

func (p *MessagesPage) renderMessagePanel() string {
	w := p.width - 4
	if w < 30 {
		w = 30
	}

	borderColor := ui.BorderColor
	if p.contentFocused {
		borderColor = ui.BrandPrimary
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 2).
		Width(w)

	// Content budget = total page height - fixed chrome around content.
	// The panel MUST fit in this budget; we clip at the end as a safety net.
	contentBudget := p.height - messagePanelChrome
	if contentBudget < 3 {
		contentBudget = 3
	}

	if p.msgLoading {
		return panel.Render(padOrClip([]string{
			lipgloss.NewStyle().Foreground(ui.SubtleColor).Italic(true).Render("Loading message..."),
		}, contentBudget))
	}
	if p.msgErr != nil {
		return panel.Render(padOrClip([]string{
			lipgloss.NewStyle().Foreground(ui.Error).Render("Error: " + p.msgErr.Error()),
		}, contentBudget))
	}
	if p.currentMsg == nil {
		return panel.Render(padOrClip([]string{
			lipgloss.NewStyle().Foreground(ui.TextFaint).Italic(true).Render("No message to display"),
		}, contentBudget))
	}

	msg := p.currentMsg
	innerW := w - 4 // account for panel padding

	titleStyle := lipgloss.NewStyle().Foreground(ui.BrandSecondary).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(ui.TextFaint)
	sectionStyle := lipgloss.NewStyle().Foreground(ui.BrandPrimary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Width(11)
	valueStyle := lipgloss.NewStyle().Foreground(ui.TextColor)

	payloadLines, kind := formatPayload(msg.Data, innerW)
	kindLabel := map[string]string{"json": "JSON", "text": "Text", "empty": "Empty"}[kind]

	title := titleStyle.Render("Message #"+utils.FormatSeq(msg.Sequence)) +
		"  " + metaStyle.Render("("+kindLabel+", "+utils.FormatBytes(uint64(len(msg.Data)))+")")

	// Build the non-payload prefix lines first so we know exactly how many
	// rows remain for the payload.
	var prefix []string
	prefix = append(prefix,
		title,
		"",
		labelStyle.Render("Subject:")+"  "+valueStyle.Render(msg.Subject),
		labelStyle.Render("Timestamp:")+"  "+valueStyle.Render(msg.Timestamp),
	)

	if len(msg.Headers) > 0 {
		prefix = append(prefix, "", sectionStyle.Render("\u2500 Headers"))
		keys := make([]string, 0, len(msg.Headers))
		for k := range msg.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		hdrKeyW := 14
		for _, k := range keys {
			if runewidth.StringWidth(k) > hdrKeyW-2 && runewidth.StringWidth(k) < 24 {
				hdrKeyW = runewidth.StringWidth(k) + 2
			}
		}
		hdrKeyStyle := lipgloss.NewStyle().Foreground(ui.SubtleColor).Width(hdrKeyW)
		for _, k := range keys {
			v := strings.Join(msg.Headers[k], ", ")
			prefix = append(prefix, hdrKeyStyle.Render(k+":")+valueStyle.Render(v))
		}
	}

	prefix = append(prefix, "", sectionStyle.Render("\u2500 Payload"))

	// Reserve one row for the scroll indicator only when there's actually
	// more payload than fits.
	needIndicator := false
	payloadBudget := contentBudget - len(prefix)
	if payloadBudget < 1 {
		// Prefix already overflows the budget \u2014 drop everything into the
		// clip step below and hope for graceful truncation.
		return panel.Render(padOrClip(prefix, contentBudget))
	}
	if len(payloadLines) > payloadBudget {
		needIndicator = true
		payloadBudget--
		if payloadBudget < 1 {
			payloadBudget = 1
		}
	}

	// Clamp scroll to the valid range for the current budget.
	maxScroll := 0
	if len(payloadLines) > payloadBudget {
		maxScroll = len(payloadLines) - payloadBudget
	}
	if p.payloadScroll > maxScroll {
		p.payloadScroll = maxScroll
	}
	if p.payloadScroll < 0 {
		p.payloadScroll = 0
	}

	start := p.payloadScroll
	end := start + payloadBudget
	if end > len(payloadLines) {
		end = len(payloadLines)
	}

	lines := prefix
	for i := start; i < end; i++ {
		lines = append(lines, valueStyle.Render(payloadLines[i]))
	}
	// Bottom-anchor: pad the payload region up to payloadBudget so the panel
	// keeps a fixed height regardless of content length.
	for i := end - start; i < payloadBudget; i++ {
		lines = append(lines, "")
	}
	if needIndicator {
		indicator := fmt.Sprintf("  [%d-%d / %d lines]  j/k scroll",
			start+1, end, len(payloadLines))
		lines = append(lines, metaStyle.Render(indicator))
	}

	return panel.Render(padOrClip(lines, contentBudget))
}

// padOrClip guarantees the joined content is exactly `budget` lines: pads with
// blanks when short, clips from the top when over (keeping the bottom visible,
// consistent with the bottom-anchor layout preference).
func padOrClip(lines []string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if len(lines) > budget {
		lines = lines[len(lines)-budget:]
	} else {
		for len(lines) < budget {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

// payloadVisibleRows returns the exact number of payload lines the panel
// will render, mirroring the budget math in renderMessagePanel so that
// PgDn/PgUp and G reach the true bottom.
func (p *MessagesPage) payloadVisibleRows() int {
	// prefix = title + blank + subject + timestamp + (headers block) + blank + section header
	prefix := 4 + 2
	if p.currentMsg != nil && len(p.currentMsg.Headers) > 0 {
		prefix += 2 + len(p.currentMsg.Headers)
	}
	rows := p.height - messagePanelChrome - prefix
	// Reserve one row for the scroll indicator when scrollable — the render
	// function does the same.
	if p.currentMsg != nil {
		innerW := p.width - 8
		if innerW < 20 {
			innerW = 20
		}
		lines, _ := formatPayload(p.currentMsg.Data, innerW)
		if len(lines) > rows {
			rows--
		}
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (p *MessagesPage) maxPayloadScroll() int {
	if p.currentMsg == nil {
		return 0
	}
	innerW := p.width - 8
	if innerW < 20 {
		innerW = 20
	}
	lines, _ := formatPayload(p.currentMsg.Data, innerW)
	visible := p.payloadVisibleRows()
	if len(lines) <= visible {
		return 0
	}
	return len(lines) - visible
}

// formatPayload returns wrapped payload lines and a kind label.
// If the payload parses as JSON it is pretty-printed with 2-space indent;
// otherwise raw text is wrapped to the given width.
func formatPayload(data string, width int) ([]string, string) {
	if width <= 0 {
		width = 60
	}
	if data == "" {
		return []string{"(empty)"}, "empty"
	}
	trimmed := strings.TrimSpace(data)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(trimmed), "", "  "); err == nil {
			return wrapText(buf.String(), width), "json"
		}
	}
	return wrapText(data, width), "text"
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			out = append(out, "")
			continue
		}
		for runewidth.StringWidth(line) > width {
			cut := 0
			w := 0
			for i, r := range line {
				rw := runewidth.RuneWidth(r)
				if w+rw > width {
					cut = i
					break
				}
				w += rw
			}
			if cut <= 0 {
				cut = len(line)
			}
			out = append(out, line[:cut])
			line = line[cut:]
		}
		out = append(out, line)
	}
	return out
}

func (p *MessagesPage) HelpText() string {
	switch p.mode {
	case ModeMessageView:
		return "h/l: Prev/Next  j/k: Scroll  y: Copy  v: Full View  Shift+drag: Select  P: Publish  d: Delete  t: Tail  r: Refresh"
	case ModeMessageDetail:
		return "j/k · PgUp/PgDn · g/G: Scroll  h/l: Prev/Next Msg  y: Copy  Shift+drag: Select  Esc/v: Back"
	case ModePublish:
		return "Tab: Next Field  Ctrl+S: Publish  Esc: Cancel"
	case ModeTail:
		if p.tail != nil && p.tail.IsFilterFocused() {
			return "Type filter · Enter: Apply  Esc: Cancel"
		}
		if p.tail != nil && p.tail.filter != "" {
			return "/: Edit Filter  Esc: Clear Filter  Space/p: Pause  c: Clear  j/k: Scroll  G: Jump Live"
		}
		return "/: Filter  Space/p: Pause  c: Clear  j/k: Scroll  G: Jump Live  Esc: Back"
	}
	return ""
}

func (p *MessagesPage) StatusText() string {
	switch p.mode {
	case ModeMessageView, ModeMessageDetail:
		if p.loading {
			return "Loading..."
		}
		if p.selectedStream == nil {
			return "No streams"
		}
		if p.msgLoading {
			return p.selectedStream.Name + " - Loading..."
		}
		prefix := ""
		if p.mode == ModeMessageDetail {
			prefix = "Detail · "
		}
		return prefix + fmt.Sprintf("%s - Seq %s/%s", p.selectedStream.Name,
			utils.FormatSeq(p.currentSeq),
			utils.FormatSeq(p.selectedStream.LastSeq))
	case ModePublish:
		if p.publish != nil {
			if p.publish.Loading() {
				return "Publishing..."
			}
			if p.publish.Success() {
				return fmt.Sprintf("Published seq %s", utils.FormatSeq(p.publish.LastSeq()))
			}
		}
		return "Publish Message"
	case ModeTail:
		if p.tail == nil {
			return "Tail"
		}
		state := "LIVE"
		if p.tail.paused {
			state = "PAUSED"
		}
		if p.tail.err != "" {
			state = "ERROR"
		}
		return fmt.Sprintf("Tail %s • %s", p.tail.stream.Name, state)
	}
	return ""
}

// streamSelectorFocusItem wires the persistent Stream selector pill into the
// focus manager. Enter opens the dropdown (delegating keyboard control to the
// selector until it closes); arrow keys escape to neighbouring items.
type streamSelectorFocusItem struct {
	page *MessagesPage
	// downTarget is the focus index to jump to on ↓ instead of walking the
	// linear list (which would land on the segment — same visual row). Zero
	// means "linear escape". Set by FocusItems.
	downTarget int
}

func (s *streamSelectorFocusItem) Focus() { s.page.selectorKeyboardFocused = true; s.page.selector.SetKeyboardFocused(true) }
func (s *streamSelectorFocusItem) Blur()  { s.page.selectorKeyboardFocused = false; s.page.selector.SetKeyboardFocused(false) }

func (s *streamSelectorFocusItem) Activate() tea.Cmd {
	s.page.selector.Open()
	return nil
}

func (s *streamSelectorFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	// Selector pill is leftmost on the top row: Left absorbs (no wrap), Right
	// escapes into the segment, Up escapes to Tabs, Down jumps to content
	// (skipping same-row neighbours).
	switch dir {
	case focus.DirLeft:
		return nil, true
	case focus.DirRight, focus.DirUp:
		return nil, false
	case focus.DirDown:
		if s.downTarget > 0 {
			return focus.Jump(s.downTarget), true
		}
		return nil, false
	}
	return nil, false
}

// messagesSegmentFocusItem wires the History↔Live segmented control into the
// focus manager. Left/Right are BOUND: pressing them toggles the mode
// immediately (matching a mouse click on the other side). At the far right
// (Live Tail with mode already ModeTail) Right escapes to the toolbar so the
// user can walk right into "+ Publish Message". Up escapes to Tabs; Down
// jumps to the message-content item so the user can scrub prev/next-seq with
// ← / → without walking through the toolbar first.
type messagesSegmentFocusItem struct {
	page *MessagesPage
	// downTarget: focus index to jump to on ↓. Zero → linear escape.
	downTarget int
}

func (s *messagesSegmentFocusItem) Focus() { s.page.segmentKeyboardFocused = true }
func (s *messagesSegmentFocusItem) Blur()  { s.page.segmentKeyboardFocused = false }

func (s *messagesSegmentFocusItem) Activate() tea.Cmd { return nil }

func (s *messagesSegmentFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	switch dir {
	case focus.DirLeft:
		if s.page.mode == ModeTail {
			s.page.exitTail()
			return nil, true
		}
		return nil, true // at leftmost — absorb, no wrap
	case focus.DirRight:
		if s.page.mode != ModeTail {
			_, cmd := s.page.openTail()
			return cmd, true
		}
		return nil, false // at rightmost, in Tail — escape to toolbar
	case focus.DirUp:
		return nil, false // escape to Tabs
	case focus.DirDown:
		if s.downTarget > 0 {
			return focus.Jump(s.downTarget), true
		}
		return nil, false // fallback: escape to next item (toolbar)
	}
	return nil, false
}

// messagesContentFocusItem owns keyboard focus for the message-content
// region below the top bar. ← / → scrub prev/next-seq (matching the h/l
// shortcuts). ↑ jumps back up to the segment; ↓ absorbs (bottom of chain).
type messagesContentFocusItem struct {
	page     *MessagesPage
	upTarget int
}

func (c *messagesContentFocusItem) Focus() { c.page.contentFocused = true }
func (c *messagesContentFocusItem) Blur()  { c.page.contentFocused = false }

func (c *messagesContentFocusItem) Activate() tea.Cmd {
	// Enter opens the full-page payload viewer.
	if c.page.currentMsg == nil {
		return nil
	}
	c.page.detailScroll = 0
	c.page.mode = ModeMessageDetail
	return nil
}

func (c *messagesContentFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	p := c.page
	switch dir {
	case focus.DirLeft:
		if p.selectedStream != nil && p.currentSeq > p.selectedStream.FirstSeq {
			p.currentSeq--
			return p.loadCurrentMsgCmd(), true
		}
		return nil, true
	case focus.DirRight:
		if p.selectedStream != nil && p.currentSeq < p.selectedStream.LastSeq {
			p.currentSeq++
			return p.loadCurrentMsgCmd(), true
		}
		return nil, true
	case focus.DirUp:
		if c.upTarget >= 0 {
			return focus.Jump(c.upTarget), true
		}
		return nil, false
	case focus.DirDown:
		return nil, true // bottom of the focus chain — absorb
	}
	return nil, false
}
