package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
	"github.com/CooDdk/freexnats/internal/ui/pages"
	"github.com/CooDdk/freexnats/pkg/utils"
)

type AppState int

const (
	StateSplash AppState = iota
	StateMain
)

type ViewMode int

const (
	ViewList ViewMode = iota
	ViewDetail
	ViewForm
)

type PageType int

const (
	PageStreams PageType = iota
	PageConsumers
	PageMessages
	PageKVStore
	PageSettings
)

var pageNames = []string{
	"Streams",
	"Consumers",
	"Messages",
	"KV Store",
	"Settings",
}

type Model struct {
	state       AppState
	width       int
	height      int
	splash      *components.SplashScreen
	initialized bool
	splashDone  bool

	natsClient *natsclient.Client
	natsURL    string
	connected  bool
	connecting bool

	statusBar *components.StatusBar
	tabs      *components.Tabs
	dialog    *components.Dialog

	// focusMgr routes ↑↓←→ + Enter through the currently focused chrome
	// region. Items are rebuilt on page-change / view-mode-change via
	// rebuildFocus so the manager always reflects the visible layout.
	// tabsFocusItem is index 0; page-specific items follow.
	focusMgr      *focus.Manager
	tabsFocusItem *pages.TabsFocusItem

	currentPage PageType
	viewMode    ViewMode

	streamsPage        *pages.StreamsPage
	streamDetailPage   *pages.StreamDetailPage
	createStreamPage   *pages.CreateStreamPage
	editStreamPage     *pages.EditStreamPage
	createConsumerPage *pages.CreateConsumerPage
	settingsPage       *pages.SettingsPage
	allConsumersPage   *pages.AllConsumersPage
	messagesPage       *pages.MessagesPage
	kvStorePage        *pages.KVStorePage
	createBucketPage   *pages.CreateKVBucketPage
	kvKeyFormPage      *pages.KVKeyFormPage

	// pendingConfirm discriminates the current dialog action so
	// handleDialogConfirm knows which destructive/mutating command to run
	// when the user presses Enter on "Yes". Reset to confirmNone whenever
	// the dialog opens for a new prompt or after the action completes.
	pendingConfirm confirmAction

	// pendingConsumerStream / pendingConsumerName snapshot the target
	// consumer at the moment the confirm dialog opens, so cursor drift
	// (arrow keys while the dialog is up would be a bug anyway) or a
	// later table refresh can't retarget the destructive op to a
	// different row.
	pendingConsumerStream string
	pendingConsumerName   string

	// pendingMessageStream / pendingMessageSeq snapshot the target of a
	// message-delete confirmation the same way pendingConsumer* does — the
	// user could theoretically navigate seq during the dialog and drift
	// off-target.
	pendingMessageStream string
	pendingMessageSeq    uint64

	// pendingBucketName snapshots the target of a KV bucket-delete confirm
	// dialog so a table refresh or arrow-key drift can't retarget the
	// destructive op to a different bucket.
	pendingBucketName string
}

// confirmAction identifies which pending confirmation the dialog is
// currently gating. Keep this narrow — every entry maps to one op in
// handleDialogConfirm.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmDeleteStream
	confirmPurgeStream
	confirmDeleteConsumer
	confirmResetConsumer
	confirmDeleteMessage
	confirmDeleteBucket
)

func NewModel(natsURL, natsUser, natsPass, natsToken string) Model {
	natsConfig := natsclient.DefaultConfig()
	if natsURL != "" {
		natsConfig.URL = natsURL
	}
	if natsUser != "" {
		natsConfig.Username = natsUser
		natsConfig.Password = natsPass
	}
	if natsToken != "" {
		natsConfig.Token = natsToken
	}
	client := natsclient.NewClient(natsConfig)

	tabItems := []components.TabItem{
		{ID: "streams", Name: "Streams"},
		{ID: "consumers", Name: "Consumers"},
		{ID: "messages", Name: "Messages"},
		{ID: "kv", Name: "KV Store"},
		{ID: "settings", Name: "Settings"},
	}

	tabs := components.NewTabs("main", tabItems)

	return Model{
		state:             StateSplash,
		splash:            components.NewSplashScreen(),
		natsClient:        client,
		natsURL:           natsConfig.URL,
		statusBar:         components.NewStatusBar(),
		tabs:              tabs,
		dialog:            components.NewDialog(),
		focusMgr:          focus.NewManager(),
		tabsFocusItem:     pages.NewTabsFocusItem(tabs, nil),
		currentPage:       PageStreams,
		viewMode:          ViewList,
		streamsPage:       pages.NewStreamsPage(client),
		settingsPage:      pages.NewSettingsPage(client),
		allConsumersPage:  pages.NewAllConsumersPage(client),
		messagesPage:      pages.NewMessagesPage(client),
		kvStorePage:       pages.NewKVStorePage(client),
	}
}

func (m Model) WithNoSplash() Model {
	m.state = StateMain
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.splash.Init(),
		m.connectNats(),
	)
}

func (m Model) connectNats() tea.Cmd {
	m.connecting = true
	return func() tea.Msg {
		err := m.natsClient.Connect()
		return NatsConnectedMsg{Err: err}
	}
}

type NatsConnectedMsg struct {
	Err error
}

type StreamDeletedMsg struct {
	Err error
}

type StreamPurgedMsg struct {
	Name string
	Err  error
}

type StreamCreatedMsg struct {
	Err error
}

type ConsumerCreatedMsg struct {
	Err error
}

type ConsumerDeletedMsg struct {
	Stream string
	Name   string
	Err    error
}

type ConsumerResetMsg struct {
	Stream string
	Name   string
	Err    error
}

type MessageDeletedMsg struct {
	Stream string
	Seq    uint64
	Err    error
}

type KVBucketDeletedMsg struct {
	Name string
	Err  error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.initialized = true
		m.updateLayout()
		if m.splashDone && m.state == StateSplash {
			m.state = StateMain
			if m.connected {
				m.rebuildFocus()
				return m, m.initCurrentPage()
			}
			return m, nil
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.state == StateMain && !m.dialog.Visible() && m.viewMode != ViewForm {
				if m.viewMode == ViewDetail {
					m.viewMode = ViewList
					return m, nil
				}
				return m, tea.Quit
			}
		}

	case components.SplashDoneMsg:
		m.splashDone = true
		if m.initialized {
			m.state = StateMain
			m.updateLayout()
			if m.connected {
				m.rebuildFocus()
				return m, m.initCurrentPage()
			}
		}
		return m, nil

	case NatsConnectedMsg:
		m.connecting = false
		if msg.Err == nil {
			m.connected = true
		}
		m.streamsPage.SetConnected(m.connected)
		m.messagesPage.SetConnected(m.connected)
		m.allConsumersPage.SetConnected(m.connected)
		m.kvStorePage.SetConnected(m.connected)

		if m.state == StateMain && m.connected {
			m.rebuildFocus()
			return m, m.initCurrentPage()
		}
		return m, nil

	case StreamDeletedMsg:
		if msg.Err != nil {
			m.dialog.Show("Error", "Failed to delete stream: "+msg.Err.Error(), components.DialogError)
			return m, nil
		}
		cmd := m.streamsPage.Init()
		return m, cmd

	case StreamPurgedMsg:
		if msg.Err != nil {
			m.dialog.Show("Error", "Failed to purge stream: "+msg.Err.Error(), components.DialogError)
			return m, nil
		}
		// Refresh the list so message counts reset to zero.
		return m, m.streamsPage.Init()

	case ConsumerDeletedMsg:
		if msg.Err != nil {
			m.dialog.Show("Error", "Failed to delete consumer: "+msg.Err.Error(), components.DialogError)
			return m, nil
		}
		return m, m.allConsumersPage.Init()

	case ConsumerResetMsg:
		if msg.Err != nil {
			m.dialog.Show("Error", "Failed to reset consumer cursor: "+msg.Err.Error(), components.DialogError)
			return m, nil
		}
		return m, m.allConsumersPage.Init()

	case MessageDeletedMsg:
		if msg.Err != nil {
			m.dialog.Show("Error", "Failed to delete message: "+msg.Err.Error(), components.DialogError)
			return m, nil
		}
		// Page-level refresh: pulls fresh stream info + auto-advances to the
		// nearest surviving seq (or exits to stream list if empty).
		return m, m.messagesPage.RefreshAfterDelete()

	case pages.KVBucketCreatedMsg:
		// Forward first so the create page flips to success/error state.
		if m.createBucketPage != nil {
			m.createBucketPage.Update(msg)
		}
		if msg.Err == nil {
			// Refresh the bucket list in the background so it's fresh when
			// the user Esc's back. The create page keeps its success banner.
			return m, m.kvStorePage.ReloadBuckets()
		}
		return m, nil

	case pages.KVKeyPutMsg:
		// Forward first so the create/edit page flips to success state.
		if m.kvKeyFormPage != nil {
			m.kvKeyFormPage.Update(msg)
		}
		if msg.Err == nil {
			// Refresh the key list so the new / updated key is current when
			// the user Esc's back to it.
			return m, m.kvStorePage.ReloadKeys()
		}
		return m, nil

	case KVBucketDeletedMsg:
		if msg.Err != nil {
			m.dialog.Show("Error", "Failed to delete KV bucket: "+msg.Err.Error(), components.DialogError)
			return m, nil
		}
		return m, m.kvStorePage.ReloadBuckets()

	case pages.StreamUpdatedMsg:
		// Forward first so the edit page flips to success/error state.
		if m.editStreamPage != nil {
			m.editStreamPage.Update(msg)
		}
		if msg.Err == nil {
			// Refresh the list in the background so subjects are current when
			// the user Esc's back. The edit page keeps its success banner.
			return m, m.streamsPage.Init()
		}
		return m, nil

	case pages.SettingsConnectMsg:
		if msg.Err != nil {
			m.settingsPage.SetConnectError(msg.Err.Error())
			return m, nil
		}
		m.natsClient.Disconnect()
		m.natsClient = msg.Client
		m.connected = true
		m.viewMode = ViewList
		m.streamDetailPage = nil
		m.createStreamPage = nil
		m.editStreamPage = nil
		m.createConsumerPage = nil
		m.streamsPage = pages.NewStreamsPage(m.natsClient)
		m.streamsPage.SetSize(m.contentWidth(), m.contentHeight())
		m.streamsPage.SetConnected(true)
		m.settingsPage = pages.NewSettingsPage(m.natsClient)
		m.settingsPage.SetSize(m.contentWidth(), m.contentHeight())
		m.settingsPage.Init()
		m.allConsumersPage = pages.NewAllConsumersPage(m.natsClient)
		m.allConsumersPage.SetSize(m.contentWidth(), m.contentHeight())
		m.allConsumersPage.SetConnected(true)
		m.messagesPage = pages.NewMessagesPage(m.natsClient)
		m.messagesPage.SetSize(m.contentWidth(), m.contentHeight())
		m.messagesPage.SetConnected(true)
		m.kvStorePage = pages.NewKVStorePage(m.natsClient)
		m.kvStorePage.SetSize(m.contentWidth(), m.contentHeight())
		m.kvStorePage.SetConnected(true)
		return m, m.streamsPage.Init()

	case components.DialogConfirmMsg:
		return m.handleDialogConfirm()

	case components.DialogCancelMsg:
		// User dismissed the confirm — reset the pending action so a later
		// "Yes" on an unrelated dialog can't accidentally fire it.
		m.pendingConfirm = confirmNone
		m.pendingConsumerStream = ""
		m.pendingConsumerName = ""
		m.pendingMessageStream = ""
		m.pendingMessageSeq = 0
		m.pendingBucketName = ""
		return m, nil

	case openStreamDetailMsg:
		if m.currentPage != PageStreams || m.viewMode != ViewList {
			return m, nil
		}
		stream := m.streamsPage.SelectedStream()
		if stream == nil {
			return m, nil
		}
		m.streamDetailPage = pages.NewStreamDetailPage(m.natsClient, stream.Name)
		m.streamDetailPage.SetSize(m.contentWidth(), m.contentHeight())
		m.viewMode = ViewDetail
		m.rebuildFocus()
		return m, m.streamDetailPage.Init()

	case pages.TabsChangedMsg:
		// Arrow-key selection change on Tabs (via focus manager). Mirrors the
		// mouse-click / Tab-key page-switch flow.
		m.currentPage = PageType(msg.Index)
		m.viewMode = ViewList
		return m, m.onPageChange()
	}

	switch m.state {
	case StateSplash:
		_, cmd := m.splash.Update(msg)
		return m, cmd

	case StateMain:
		return m.updateMain(msg)
	}

	return m, nil
}

func (m Model) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dialog.Visible() {
		_, cmd := m.dialog.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Route by Y region: the tab row (and everything above it) is owned
		// exclusively by Tabs; anything below falls through to the current
		// page. This prevents overlap between Tabs' ±1 tolerance and a
		// toolbar's ±1 tolerance from misfiring across widget boundaries.
		//
		// TopY() returns 0 before the first View() sets it — in that case
		// we skip tab routing entirely and forward, which is safe because
		// mouse events cannot arrive before the first render anyway.
		//
		// Tabs are clickable in every viewMode. A click while in ViewForm
		// or ViewDetail acts as "cancel current sub-mode, then switch" —
		// matching the Esc + digit-key keyboard flow.
		tabRowY := m.tabs.TopY()
		// Strict two-row band: only Y == tabRowY (the tab row itself) and
		// Y == tabRowY-1 (one-row Y-underreport tolerance matching Tabs.HitTest)
		// belong to Tabs. Anything above is the header (not interactive) and
		// anything below is page content — do not absorb those or a click on
		// a lower widget with an off-by-2 Y report can misfire as a tab switch.
		if tabRowY > 0 && msg.Y >= tabRowY-1 && msg.Y <= tabRowY {
			if idx := m.tabs.HitTest(msg); idx >= 0 {
				if m.viewMode == ViewForm {
					m.createStreamPage = nil
					m.editStreamPage = nil
					m.createConsumerPage = nil
					m.createBucketPage = nil
					m.kvKeyFormPage = nil
				}
				m.viewMode = ViewList
				if idx != int(m.currentPage) {
					m.tabs.SetSelected(idx)
					m.currentPage = PageType(idx)
					return m, m.onPageChange()
				}
				m.rebuildFocus()
				return m, nil
			}
			// Header or non-hit tab click — drop; do not forward down.
			return m, nil
		}

	case focus.JumpMsg:
		// A focus item asked to hand focus to a specific index (used by
		// 2D layouts where ↓ needs to leap past same-row neighbours). The
		// manager blurs the current item and focuses the target; if the
		// index is out of range SetCurrent is a no-op.
		m.focusMgr.SetCurrent(msg.Target)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			switch m.viewMode {
			case ViewForm:
				m.viewMode = ViewList
				kvForm := m.currentPage == PageKVStore
				kvKeyForm := m.kvKeyFormPage != nil
				m.createStreamPage = nil
				m.editStreamPage = nil
				m.createConsumerPage = nil
				m.createBucketPage = nil
				m.kvKeyFormPage = nil
				m.rebuildFocus()
				if kvForm {
					// Refresh the appropriate KV list. After a key-form exit
					// the user lands back on the key list; after a bucket-form
					// exit they land back on the bucket list.
					if kvKeyForm {
						return m, m.kvStorePage.ReloadKeys()
					}
					return m, m.kvStorePage.ReloadBuckets()
				}
				cmd := m.streamsPage.Init()
				return m, cmd
			case ViewDetail:
				m.viewMode = ViewList
				m.rebuildFocus()
				return m, nil
			}

		case "tab":
			if m.viewMode != ViewForm {
				m.tabs.Next()
				m.currentPage = PageType(m.tabs.Selected())
				return m, m.onPageChange()
			}

		case "shift+tab":
			if m.viewMode != ViewForm {
				m.tabs.Prev()
				m.currentPage = PageType(m.tabs.Selected())
				return m, m.onPageChange()
			}

		case "up", "down", "left", "right", "enter":
			// Route through the focus manager first. It's only wired for
			// pages that have called rebuildFocus with a non-trivial item
			// list; other pages fall through unchanged.
			if m.focusMgr.Current() >= 0 && m.focusRouted() {
				if cmd, handled := m.focusMgr.HandleKey(msg.String()); handled {
					// Any arrow or Enter may have triggered a state change
					// that reshapes the focus list (KV mode flip, sub-tab
					// switch under StreamDetail, etc.). Rebuild so the new
					// widget layout is what subsequent arrow keys act on.
					m.rebuildFocus()
					return m, cmd
				}
			}
			// Legacy Enter → detail flow (still needed for pages not yet
			// wired to the focus manager).
			if msg.String() == "enter" && m.currentPage == PageStreams && m.viewMode == ViewList {
				// Focus manager already handled this above when routed; the
				// legacy path stays here in case focus routing is off.
				stream := m.streamsPage.SelectedStream()
				if stream != nil {
					m.streamDetailPage = pages.NewStreamDetailPage(m.natsClient, stream.Name)
					m.streamDetailPage.SetSize(m.contentWidth(), m.contentHeight())
					m.viewMode = ViewDetail
					m.rebuildFocus()
					cmd := m.streamDetailPage.Init()
					return m, cmd
				}
			}

		case "n":
			if m.currentPage == PageStreams && m.viewMode == ViewList && m.connected {
				m.createStreamPage = pages.NewCreateStreamPage(m.natsClient)
				m.createStreamPage.SetSize(m.contentWidth(), m.contentHeight())
				m.viewMode = ViewForm
				return m, nil
			}
			if m.currentPage == PageStreams && m.viewMode == ViewDetail && m.streamDetailPage != nil && m.streamDetailPage.ActiveTab() == 2 && m.connected {
				m.createConsumerPage = pages.NewCreateConsumerPage(m.natsClient, m.streamDetailPage.StreamName())
				m.createConsumerPage.SetSize(m.contentWidth(), m.contentHeight())
				m.viewMode = ViewForm
				return m, nil
			}
			if m.currentPage == PageKVStore && m.viewMode == ViewList && m.kvStorePage.Mode() == pages.ModeBucketList && m.connected {
				m.createBucketPage = pages.NewCreateKVBucketPage(m.natsClient)
				m.createBucketPage.SetSize(m.contentWidth(), m.contentHeight())
				m.viewMode = ViewForm
				m.rebuildFocus()
				return m, nil
			}
			if m.currentPage == PageKVStore && m.viewMode == ViewList && m.kvStorePage.Mode() == pages.ModeKeyList && m.connected {
				bucket := m.kvStorePage.SelectedBucket()
				if bucket != "" {
					m.kvKeyFormPage = pages.NewCreateKVKeyPage(m.natsClient, bucket)
					m.kvKeyFormPage.SetSize(m.contentWidth(), m.contentHeight())
					m.viewMode = ViewForm
					m.rebuildFocus()
					return m, nil
				}
			}

		case "e":
			if m.currentPage == PageStreams && m.viewMode == ViewList && m.connected {
				stream := m.streamsPage.SelectedStream()
				if stream != nil {
					m.editStreamPage = pages.NewEditStreamPage(m.natsClient, stream)
					m.editStreamPage.SetSize(m.contentWidth(), m.contentHeight())
					m.viewMode = ViewForm
					return m, nil
				}
			}
			if m.currentPage == PageKVStore && m.viewMode == ViewList && m.kvStorePage.Mode() == pages.ModeValueView && m.connected {
				entry := m.kvStorePage.LoadedEntry()
				bucket := m.kvStorePage.SelectedBucket()
				if entry != nil && bucket != "" {
					m.kvKeyFormPage = pages.NewEditKVKeyPage(m.natsClient, bucket, entry.Key, entry.Value)
					m.kvKeyFormPage.SetSize(m.contentWidth(), m.contentHeight())
					m.viewMode = ViewForm
					m.rebuildFocus()
					return m, nil
				}
			}

		case "d":
			if m.currentPage == PageStreams && m.viewMode == ViewList && m.connected {
				stream := m.streamsPage.SelectedStream()
				if stream != nil {
					m.pendingConfirm = confirmDeleteStream
					m.dialog.Show(
						"Delete Stream",
						"Are you sure you want to delete stream '"+stream.Name+"'? This cannot be undone.",
						components.DialogConfirm,
					)
					return m, func() tea.Msg {
						return DeleteStreamConfirmMsg{Name: stream.Name}
					}
				}
			}
			if m.currentPage == PageConsumers && m.viewMode == ViewList && m.connected {
				entry := m.allConsumersPage.SelectedEntry()
				if entry != nil && entry.ConsumerInfo != nil {
					m.pendingConfirm = confirmDeleteConsumer
					m.pendingConsumerStream = entry.StreamName
					m.pendingConsumerName = entry.ConsumerInfo.Name
					m.dialog.Show(
						"Delete Consumer",
						"Delete consumer '"+entry.ConsumerInfo.Name+"' on stream '"+entry.StreamName+"'? This cannot be undone.",
						components.DialogConfirm,
					)
					return m, nil
				}
			}
			if m.currentPage == PageMessages && m.connected && m.messagesPage.HasSelectedMessage() {
				stream, seq := m.messagesPage.SelectedStreamAndSeq()
				if stream != "" {
					m.pendingConfirm = confirmDeleteMessage
					m.pendingMessageStream = stream
					m.pendingMessageSeq = seq
					m.dialog.Show(
						"Delete Message",
						"Delete message seq "+utils.FormatSeq(seq)+" from stream '"+stream+"'? This cannot be undone.",
						components.DialogConfirm,
					)
					return m, nil
				}
			}
			if m.currentPage == PageKVStore && m.viewMode == ViewList && m.kvStorePage.Mode() == pages.ModeBucketList && m.connected {
				b := m.kvStorePage.SelectedBucketInfo()
				if b != nil {
					m.pendingConfirm = confirmDeleteBucket
					m.pendingBucketName = b.Name
					m.dialog.Show(
						"Delete KV Bucket",
						"Delete KV bucket '"+b.Name+"'? All keys and history will be lost. This cannot be undone.",
						components.DialogConfirm,
					)
					return m, nil
				}
			}

		case "z":
			if m.currentPage == PageConsumers && m.viewMode == ViewList && m.connected {
				entry := m.allConsumersPage.SelectedEntry()
				if entry != nil && entry.ConsumerInfo != nil {
					m.pendingConfirm = confirmResetConsumer
					m.pendingConsumerStream = entry.StreamName
					m.pendingConsumerName = entry.ConsumerInfo.Name
					m.dialog.Show(
						"Reset Consumer Cursor",
						"Reset the cursor for consumer '"+entry.ConsumerInfo.Name+"' on stream '"+entry.StreamName+"'? The consumer will be recreated with its current config and playback will restart per its DeliverPolicy.",
						components.DialogConfirm,
					)
					return m, nil
				}
			}

		case "p":
			if m.currentPage == PageStreams && m.viewMode == ViewList && m.connected {
				stream := m.streamsPage.SelectedStream()
				if stream != nil {
					m.pendingConfirm = confirmPurgeStream
					m.dialog.Show(
						"Purge Stream",
						"Delete all messages in stream '"+stream.Name+"'? The stream itself remains but every message is dropped. This cannot be undone.",
						components.DialogConfirm,
					)
					return m, nil
				}
			}
		}
	}

	var cmd tea.Cmd
	switch m.currentPage {
	case PageStreams:
		switch m.viewMode {
		case ViewList:
			_, cmd = m.streamsPage.Update(msg)
		case ViewDetail:
			if m.streamDetailPage != nil {
				_, cmd = m.streamDetailPage.Update(msg)
			}
		case ViewForm:
			if m.createStreamPage != nil {
				_, cmd = m.createStreamPage.Update(msg)
			} else if m.editStreamPage != nil {
				_, cmd = m.editStreamPage.Update(msg)
			} else if m.createConsumerPage != nil {
				_, cmd = m.createConsumerPage.Update(msg)
			}
		}
	case PageSettings:
		_, cmd = m.settingsPage.Update(msg)
	case PageConsumers:
		_, cmd = m.allConsumersPage.Update(msg)
	case PageMessages:
		_, cmd = m.messagesPage.Update(msg)
	case PageKVStore:
		if m.viewMode == ViewForm && m.kvKeyFormPage != nil {
			_, cmd = m.kvKeyFormPage.Update(msg)
		} else if m.viewMode == ViewForm && m.createBucketPage != nil {
			_, cmd = m.createBucketPage.Update(msg)
		} else {
			_, cmd = m.kvStorePage.Update(msg)
		}
	}

	return m, cmd
}

type DeleteStreamConfirmMsg struct {
	Name string
}

func (m Model) handleDialogConfirm() (tea.Model, tea.Cmd) {
	action := m.pendingConfirm
	m.pendingConfirm = confirmNone
	switch action {
	case confirmDeleteStream:
		stream := m.streamsPage.SelectedStream()
		if stream != nil {
			return m, m.deleteStreamCmd(stream.Name)
		}
	case confirmPurgeStream:
		stream := m.streamsPage.SelectedStream()
		if stream != nil {
			return m, m.purgeStreamCmd(stream.Name)
		}
	case confirmDeleteConsumer:
		stream, name := m.pendingConsumerStream, m.pendingConsumerName
		m.pendingConsumerStream, m.pendingConsumerName = "", ""
		if stream != "" && name != "" {
			return m, m.deleteConsumerCmd(stream, name)
		}
	case confirmResetConsumer:
		stream, name := m.pendingConsumerStream, m.pendingConsumerName
		m.pendingConsumerStream, m.pendingConsumerName = "", ""
		if stream != "" && name != "" {
			return m, m.resetConsumerCmd(stream, name)
		}
	case confirmDeleteMessage:
		stream, seq := m.pendingMessageStream, m.pendingMessageSeq
		m.pendingMessageStream, m.pendingMessageSeq = "", 0
		if stream != "" && seq > 0 {
			return m, m.deleteMessageCmd(stream, seq)
		}
	case confirmDeleteBucket:
		name := m.pendingBucketName
		m.pendingBucketName = ""
		if name != "" {
			return m, m.deleteBucketCmd(name)
		}
	}
	return m, nil
}

func (m Model) deleteStreamCmd(name string) tea.Cmd {
	return func() tea.Msg {
		err := m.natsClient.DeleteStream(name)
		return StreamDeletedMsg{Err: err}
	}
}

func (m Model) purgeStreamCmd(name string) tea.Cmd {
	return func() tea.Msg {
		err := m.natsClient.PurgeStream(name)
		return StreamPurgedMsg{Name: name, Err: err}
	}
}

func (m Model) deleteConsumerCmd(stream, name string) tea.Cmd {
	return func() tea.Msg {
		err := m.natsClient.DeleteConsumer(stream, name)
		return ConsumerDeletedMsg{Stream: stream, Name: name, Err: err}
	}
}

func (m Model) resetConsumerCmd(stream, name string) tea.Cmd {
	return func() tea.Msg {
		err := m.natsClient.ResetConsumer(stream, name)
		return ConsumerResetMsg{Stream: stream, Name: name, Err: err}
	}
}

func (m Model) deleteMessageCmd(stream string, seq uint64) tea.Cmd {
	return func() tea.Msg {
		err := m.natsClient.DeleteStreamMessage(stream, seq)
		return MessageDeletedMsg{Stream: stream, Seq: seq, Err: err}
	}
}

func (m Model) deleteBucketCmd(name string) tea.Cmd {
	return func() tea.Msg {
		err := m.natsClient.DeleteKVBucket(name)
		return KVBucketDeletedMsg{Name: name, Err: err}
	}
}

func (m Model) onPageChange() tea.Cmd {
	m.rebuildFocus()
	switch m.currentPage {
	case PageStreams:
		if m.connected {
			return m.streamsPage.Init()
		}
	case PageSettings:
		return m.settingsPage.Init()
	case PageConsumers:
		if m.connected {
			return m.allConsumersPage.Init()
		}
	case PageMessages:
		if m.connected {
			return m.messagesPage.Init()
		}
	case PageKVStore:
		if m.connected {
			return m.kvStorePage.Init()
		}
	}
	return nil
}

// rebuildFocus reassembles the focus.Manager's item list to reflect the
// current page + view mode. Tabs stays at index 0 (always focusable);
// content-area items come from the page's FocusItems(). Preserves the
// currently-focused index across rebuilds when it's still in range, so
// arrow-key navigation and Enter-triggered mode changes don't reset focus
// back to the toolbar every keystroke.
func (m Model) rebuildFocus() {
	items := []focus.Item{m.tabsFocusItem}
	switch m.currentPage {
	case PageStreams:
		if m.viewMode == ViewList {
			items = append(items, m.streamsPage.FocusItems(func() tea.Cmd {
				// Enter on the table row = same as pressing Enter globally
				// on the Streams page (opens detail view).
				stream := m.streamsPage.SelectedStream()
				if stream == nil {
					return nil
				}
				return func() tea.Msg {
					return openStreamDetailMsg{name: stream.Name}
				}
			})...)
		} else if m.viewMode == ViewForm {
			switch {
			case m.createStreamPage != nil:
				items = append(items, m.createStreamPage.FocusItems()...)
			case m.editStreamPage != nil:
				items = append(items, m.editStreamPage.FocusItems()...)
			case m.createConsumerPage != nil:
				items = append(items, m.createConsumerPage.FocusItems()...)
			}
		} else if m.viewMode == ViewDetail && m.streamDetailPage != nil {
			items = append(items, m.streamDetailPage.FocusItems()...)
		}
	case PageConsumers:
		items = append(items, m.allConsumersPage.FocusItems(func() tea.Cmd {
			// Enter on a consumer row shows the inline detail overlay
			// (same as pressing 'v' / 'enter' in legacy key handling).
			m.allConsumersPage.ShowOverlay()
			return nil
		})...)
	case PageKVStore:
		if m.viewMode == ViewForm && m.kvKeyFormPage != nil {
			items = append(items, m.kvKeyFormPage.FocusItems()...)
		} else if m.viewMode == ViewForm && m.createBucketPage != nil {
			items = append(items, m.createBucketPage.FocusItems()...)
		} else {
			switch m.kvStorePage.Mode() {
			case pages.ModeBucketList:
				items = append(items, m.kvStorePage.FocusItems(func() tea.Cmd {
					return m.kvStorePage.ActivateSelectedBucket()
				})...)
			case pages.ModeKeyList:
				items = append(items, m.kvStorePage.FocusItems(func() tea.Cmd {
					return m.kvStorePage.ActivateSelectedKey()
				})...)
			}
		}
	case PageMessages:
		items = append(items, m.messagesPage.FocusItems(len(items))...)
	case PageSettings:
		items = append(items, m.settingsPage.FocusItems()...)
	}
	// Preserve focus across rebuilds when the target index is still valid;
	// otherwise land on the first content item (or Tabs when the page
	// contributed nothing).
	want := m.focusMgr.Current()
	if want < 0 || want >= len(items) {
		want = 1
		if len(items) < 2 {
			want = 0
		}
	}
	m.focusMgr.SetItems(items, want)
}

// openStreamDetailMsg is emitted by the streams-page focus item's Activate
// (Enter on a table row) so app.go can transition to ViewDetail using the
// same code path as pressing Enter globally.
type openStreamDetailMsg struct{ name string }

// focusRouted reports whether the current page + view mode has been wired
// to route arrow keys through focus.Manager. Pages / modes not yet
// migrated fall through to their legacy per-widget key handlers so
// keyboard behavior stays unchanged during the Phase 2 rollout.
func (m Model) focusRouted() bool {
	switch m.currentPage {
	case PageStreams:
		if m.viewMode == ViewList {
			return true
		}
		if m.viewMode == ViewForm {
			switch {
			case m.createStreamPage != nil:
				return m.createStreamPage.FocusRouted()
			case m.editStreamPage != nil:
				return m.editStreamPage.FocusRouted()
			case m.createConsumerPage != nil:
				return m.createConsumerPage.FocusRouted()
			}
		}
		if m.viewMode == ViewDetail && m.streamDetailPage != nil {
			return m.streamDetailPage.FocusRouted()
		}
		return false
	case PageConsumers:
		return m.allConsumersPage.FocusRouted()
	case PageKVStore:
		if m.viewMode == ViewForm && m.kvKeyFormPage != nil {
			return m.kvKeyFormPage.FocusRouted()
		}
		if m.viewMode == ViewForm && m.createBucketPage != nil {
			return m.createBucketPage.FocusRouted()
		}
		return m.kvStorePage.FocusRouted()
	case PageMessages:
		return m.messagesPage.FocusRouted()
	case PageSettings:
		return m.settingsPage.FocusRouted()
	}
	return false
}

func (m Model) initCurrentPage() tea.Cmd {
	switch m.currentPage {
	case PageStreams:
		return m.streamsPage.Init()
	case PageConsumers:
		return m.allConsumersPage.Init()
	case PageMessages:
		return m.messagesPage.Init()
	case PageKVStore:
		return m.kvStorePage.Init()
	case PageSettings:
		return m.settingsPage.Init()
	default:
		return nil
	}
}

const (
	tabBarRows      = 3 // tab row + 2 blank rows below (2nd blank is the "dead zone" absorbing 1-row mouse-Y underreport on Windows terminals)
	contentSidePad  = 2 // left/right breathing room around content
	chromeRowsExtra = 14
)

func (m Model) contentHeight() int {
	return m.height - chromeRowsExtra - tabBarRows
}

func (m Model) contentWidth() int {
	w := m.width - contentSidePad*2
	if w < 20 {
		w = 20
	}
	return w
}

// applyToolbarOrigins pushes the content column's absolute top-left down to
// each page's toolbar(s) so hit testing can be coordinate-based rather than
// depending on bubblezone marks (which get perturbed by outer lipgloss).
// Form pages (create/edit/settings) share the same origin so their input
// rows and Cancel/Submit buttons can be hit-tested by absolute coordinates.
func (m Model) applyToolbarOrigins(x, y int) {
	m.streamsPage.SetToolbarOrigin(x, y)
	m.allConsumersPage.SetToolbarOrigin(x, y)
	m.messagesPage.SetToolbarOrigin(x, y)
	m.kvStorePage.SetToolbarOrigin(x, y)
	m.settingsPage.SetContentOrigin(x, y)
	if m.streamDetailPage != nil {
		m.streamDetailPage.SetToolbarOrigin(x, y)
	}
	if m.createStreamPage != nil {
		m.createStreamPage.SetContentOrigin(x, y)
	}
	if m.editStreamPage != nil {
		m.editStreamPage.SetContentOrigin(x, y)
	}
	if m.createConsumerPage != nil {
		m.createConsumerPage.SetContentOrigin(x, y)
	}
	if m.createBucketPage != nil {
		m.createBucketPage.SetContentOrigin(x, y)
	}
	if m.kvKeyFormPage != nil {
		m.kvKeyFormPage.SetContentOrigin(x, y)
	}
}

func (m Model) updateLayout() {
	contentWidth := m.contentWidth()
	contentHeight := m.contentHeight()

	m.statusBar.SetWidth(m.width)
	m.streamsPage.SetSize(contentWidth, contentHeight)
	m.settingsPage.SetSize(contentWidth, contentHeight)
	m.allConsumersPage.SetSize(contentWidth, contentHeight)
	m.messagesPage.SetSize(contentWidth, contentHeight)
	m.kvStorePage.SetSize(contentWidth, contentHeight)
	if m.streamDetailPage != nil {
		m.streamDetailPage.SetSize(contentWidth, contentHeight)
	}
	if m.createStreamPage != nil {
		m.createStreamPage.SetSize(contentWidth, contentHeight)
	}
	if m.editStreamPage != nil {
		m.editStreamPage.SetSize(contentWidth, contentHeight)
	}
	if m.createConsumerPage != nil {
		m.createConsumerPage.SetSize(contentWidth, contentHeight)
	}
	if m.createBucketPage != nil {
		m.createBucketPage.SetSize(contentWidth, contentHeight)
	}
	if m.kvKeyFormPage != nil {
		m.kvKeyFormPage.SetSize(contentWidth, contentHeight)
	}
}

func (m Model) View() string {
	var v string
	switch m.state {
	case StateSplash:
		v = m.splash.View(m.width, m.height)
	case StateMain:
		if !m.initialized {
			v = m.splash.View(m.width, m.height)
		} else {
			v = m.viewMain()
		}
	}
	return zone.Scan(v)
}

func (m Model) viewMain() string {
	logoArea := components.LogoWithSubtitle()
	logoCentered := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(logoArea)

	connInfo := m.renderConnInfo()
	connCentered := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		PaddingBottom(1).
		Render(connInfo)

	headerBlock := lipgloss.JoinVertical(lipgloss.Center,
		logoCentered,
		connCentered,
	)

	divider := lipgloss.NewStyle().
		Foreground(ui.BgLighter).
		Render(stringsRepeat("\u2500", m.width))

	// Keep tab selection in sync with the model's current page (e.g. after
	// programmatic page switches).
	m.tabs.SetSelected(int(m.currentPage))

	// Tabs sit on their own row, absolute Y = header rows + divider row.
	tabsTopY := lipgloss.Height(headerBlock) + lipgloss.Height(divider)
	m.tabs.SetTopLeft(0, tabsTopY)
	tabsRow := m.tabs.View()

	// Content column starts contentSidePad columns in, immediately below the
	// tab row + one blank spacer row.
	contentTopX := contentSidePad
	// +2 rows: one blank + one extra dead row between tabs and page content.
	// Under Windows terminals that under-report mouse Y by 1 row, a toolbar
	// click lands on the dead row (no widget) instead of leaking into Tabs'
	// hit-test region — see internal/ui/components/tabs.go and toolbar.go.
	contentTopY := tabsTopY + m.tabs.Height() + 2
	m.applyToolbarOrigins(contentTopX, contentTopY)

	content := m.renderCurrentPage()
	// Pin the content area to the full computed height so short pages don't
	// let the layout collapse and cause the visible frame to jump when the
	// user switches tabs. MaxHeight caps overflow: if a page renders more
	// lines than contentHeight (Settings form, tall dialogs), we clip the
	// bottom rather than let the total view exceed the terminal and scroll
	// the LOGO off the top.
	paddedContent := lipgloss.NewStyle().
		PaddingLeft(contentSidePad).
		PaddingRight(contentSidePad).
		Height(m.contentHeight()).
		MaxHeight(m.contentHeight()).
		Render(content)

	m.statusBar.SetContent(
		m.currentPageStatusText(),
		m.currentPageHelpText(),
	)
	statusBar := m.statusBar.View()

	view := lipgloss.JoinVertical(lipgloss.Left,
		headerBlock,
		divider,
		tabsRow,
		"",
		"",
		paddedContent,
		statusBar,
	)

	if m.dialog.Visible() {
		dialog := m.dialog.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	// Hard-cap the whole frame at terminal height. If a page overflows its
	// content budget (e.g. tall form on a short terminal), we'd rather clip
	// the bottom than have the terminal scroll and drop the LOGO off the top.
	return lipgloss.NewStyle().MaxHeight(m.height).Render(view)
}

func stringsRepeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

func (m Model) renderConnInfo() string {
	statusText := "Disconnected"
	statusColor := ui.Error

	if m.connecting {
		statusText = "Connecting..."
		statusColor = ui.Warning
	} else if m.connected {
		statusText = "Connected"
		statusColor = ui.Success
	}

	dot := lipgloss.NewStyle().
		Foreground(statusColor).
		Render("●")

	status := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true).
		Render(statusText)

	url := lipgloss.NewStyle().
		Foreground(ui.TextMuted).
		Render(m.natsClient.ServerURL())

	parts := []string{dot + " " + status, "  ", url}

	if m.connected {
		version := lipgloss.NewStyle().
			Foreground(ui.TextFaint).
			Render("v" + m.natsClient.ServerVersion())
		parts = append(parts, "  │  ", version)
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m Model) renderCurrentPage() string {
	switch m.currentPage {
	case PageStreams:
		switch m.viewMode {
		case ViewDetail:
			if m.streamDetailPage != nil {
				return m.streamDetailPage.View()
			}
		case ViewForm:
			if m.createStreamPage != nil {
				return m.createStreamPage.View()
			}
			if m.editStreamPage != nil {
				return m.editStreamPage.View()
			}
			if m.createConsumerPage != nil {
				return m.createConsumerPage.View()
			}
		default:
			return m.streamsPage.View()
		}
	case PageConsumers:
		return m.allConsumersPage.View()
	case PageMessages:
		return m.messagesPage.View()
	case PageKVStore:
		if m.viewMode == ViewForm && m.kvKeyFormPage != nil {
			return m.kvKeyFormPage.View()
		}
		if m.viewMode == ViewForm && m.createBucketPage != nil {
			return m.createBucketPage.View()
		}
		return m.kvStorePage.View()
	case PageSettings:
		return m.settingsPage.View()
	}
	return ""
}

func (m Model) currentPageStatusText() string {
	switch m.currentPage {
	case PageStreams:
		switch m.viewMode {
		case ViewDetail:
			if m.streamDetailPage != nil {
				return m.streamDetailPage.StatusText()
			}
		case ViewForm:
			if m.createStreamPage != nil {
				return m.createStreamPage.StatusText()
			}
			if m.editStreamPage != nil {
				return m.editStreamPage.StatusText()
			}
			if m.createConsumerPage != nil {
				return m.createConsumerPage.StatusText()
			}
		default:
			return m.streamsPage.StatusText()
		}
	case PageSettings:
		return m.settingsPage.StatusText()
	case PageConsumers:
		return m.allConsumersPage.StatusText()
	case PageMessages:
		return m.messagesPage.StatusText()
	case PageKVStore:
		if m.viewMode == ViewForm && m.kvKeyFormPage != nil {
			return m.kvKeyFormPage.StatusText()
		}
		if m.viewMode == ViewForm && m.createBucketPage != nil {
			return m.createBucketPage.StatusText()
		}
		return m.kvStorePage.StatusText()
	}
	return pageNames[m.currentPage]
}

func (m Model) currentPageHelpText() string {
	if m.dialog.Visible() {
		return "Enter: Confirm  Esc: Cancel"
	}
	switch m.currentPage {
	case PageStreams:
		switch m.viewMode {
		case ViewDetail:
			return withTabSwitch(m.streamDetailPage.HelpText())
		case ViewForm:
			if m.createStreamPage != nil {
				return m.createStreamPage.HelpText()
			}
			if m.editStreamPage != nil {
				return m.editStreamPage.HelpText()
			}
			if m.createConsumerPage != nil {
				return m.createConsumerPage.HelpText()
			}
		default:
			return withTabSwitch(m.streamsPage.HelpText())
		}
	case PageSettings:
		return m.settingsPage.HelpText()
	case PageConsumers:
		return withTabSwitch(m.allConsumersPage.HelpText())
	case PageMessages:
		return withTabSwitch(m.messagesPage.HelpText())
	case PageKVStore:
		if m.viewMode == ViewForm && m.kvKeyFormPage != nil {
			return m.kvKeyFormPage.HelpText()
		}
		if m.viewMode == ViewForm && m.createBucketPage != nil {
			return m.createBucketPage.HelpText()
		}
		return withTabSwitch(m.kvStorePage.HelpText())
	}
	return "q: Quit  Tab: Switch Page"
}

// withTabSwitch appends the "Tab: Switch" tab-cycle hint only when the
// page's own help doesn't already claim Tab for a local purpose.
func withTabSwitch(help string) string {
	if help == "" {
		return "Tab: Switch"
	}
	if containsTabBinding(help) {
		return help
	}
	return help + "  Tab: Switch"
}

func containsTabBinding(s string) bool {
	// Any "Tab:" or "Tab/" prefix means Tab is already claimed for a local
	// binding (e.g. Tab: Next Field, Tab/Shift+Tab switch).
	for i := 0; i+3 < len(s); i++ {
		if s[i] == 'T' && s[i+1] == 'a' && s[i+2] == 'b' && (s[i+3] == ':' || s[i+3] == '/') {
			return true
		}
	}
	return false
}

