// focus_adapters.go: Item wrappers so page-level chrome (Tabs, Toolbar,
// TableList) can plug into focus.Manager without leaking bubbletea Cmds
// into the low-level widgets. Adapters are thin — they translate arrow
// keys into widget-native calls and produce Cmds that page/app code
// already knows how to handle.
package pages

import (
	tea "github.com/charmbracelet/bubbletea"

	natsclient "github.com/CooDdk/freexnats/internal/nats"
	"github.com/CooDdk/freexnats/internal/ui/components"
	"github.com/CooDdk/freexnats/internal/ui/focus"
)

// TabsFocusItem wraps a Tabs widget. Left/Right cycle the selection and
// emit a TabsChangedMsg so app.go can trigger the same page-change flow
// as clicking a tab. Up/Down escape (Tabs sits at the top of the page).
type TabsFocusItem struct {
	tabs   *components.Tabs
	notify func() tea.Cmd // called after selection changes; returns a Cmd
}

// TabsChangedMsg is emitted when arrow keys move Tabs' selection. app.go
// listens and runs the same "switch page" flow as a mouse click.
type TabsChangedMsg struct{ Index int }

func NewTabsFocusItem(tabs *components.Tabs, notify func() tea.Cmd) *TabsFocusItem {
	return &TabsFocusItem{tabs: tabs, notify: notify}
}

func (t *TabsFocusItem) Focus()      { t.tabs.Focus() }
func (t *TabsFocusItem) Blur()       { t.tabs.Blur() }
func (t *TabsFocusItem) Activate() tea.Cmd { return nil } // Enter on already-selected tab = no-op

func (t *TabsFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	switch dir {
	case focus.DirLeft:
		if t.tabs.Selected() == 0 {
			return nil, true // absorb — no wrap
		}
		t.tabs.Prev()
		return t.emit(), true
	case focus.DirRight:
		if t.tabs.Selected() == t.tabs.Count()-1 {
			return nil, true
		}
		t.tabs.Next()
		return t.emit(), true
	case focus.DirUp:
		return nil, false // at-edge; escape (no-op since Tabs is at the top)
	case focus.DirDown:
		return nil, false // at-edge; escape to first content item
	}
	return nil, false
}

func (t *TabsFocusItem) emit() tea.Cmd {
	idx := t.tabs.Selected()
	notify := t.notify
	return func() tea.Msg {
		if notify != nil {
			// The caller's Cmd is fire-and-forget; we just emit the msg for
			// state-machine coherence.
			_ = notify()
		}
		return TabsChangedMsg{Index: idx}
	}
}

// ToolbarFocusItem wraps a Toolbar. Left/Right walk between enabled
// buttons; Enter fires the focused action via the supplied activate
// callback (which returns the same Cmd as a mouse click on that button).
type ToolbarFocusItem struct {
	toolbar  *components.Toolbar
	activate func(id string) tea.Cmd
}

func NewToolbarFocusItem(tb *components.Toolbar, activate func(id string) tea.Cmd) *ToolbarFocusItem {
	return &ToolbarFocusItem{toolbar: tb, activate: activate}
}

func (t *ToolbarFocusItem) Focus() {
	// Idempotent: only pick a starting button if the currently focused
	// slot is invalid or disabled. Blindly calling FocusFirstEnabled here
	// would clobber the user's cursor position on every rebuildFocus tick
	// (which fires after every arrow keystroke), pinning the toolbar
	// selection to the first button.
	a := t.toolbar.ActionAtFocus()
	if a.ID == "" || a.Disabled {
		t.toolbar.FocusFirstEnabled()
	}
	t.toolbar.Focus()
}
func (t *ToolbarFocusItem) Blur() { t.toolbar.Blur() }

func (t *ToolbarFocusItem) Activate() tea.Cmd {
	a := t.toolbar.ActionAtFocus()
	if a.ID == "" || a.Disabled || t.activate == nil {
		return nil
	}
	return t.activate(a.ID)
}

func (t *ToolbarFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	switch dir {
	case focus.DirLeft:
		if t.toolbar.StepFocus(-1) {
			return nil, true
		}
		return nil, false // at leftmost — escape to previous focus item
	case focus.DirRight:
		if t.toolbar.StepFocus(1) {
			return nil, true
		}
		return nil, false // at rightmost — escape to next focus item
	case focus.DirUp, focus.DirDown:
		return nil, false // escape vertically
	}
	return nil, false
}

// TableListFocusItem wraps a TableList. Up/Down move the row cursor and
// only escape when at the top / bottom row. Left/Right always escape
// (tables have no horizontal cursor). Enter fires the supplied activate
// callback (typically "open detail overlay" or "enter detail page").
type TableListFocusItem struct {
	table    *components.TableList
	activate func() tea.Cmd
}

func NewTableListFocusItem(tbl *components.TableList, activate func() tea.Cmd) *TableListFocusItem {
	return &TableListFocusItem{table: tbl, activate: activate}
}

func (t *TableListFocusItem) Focus() { t.table.Focus() }
func (t *TableListFocusItem) Blur()  { t.table.Blur() }

func (t *TableListFocusItem) Activate() tea.Cmd {
	if t.activate == nil {
		return nil
	}
	return t.activate()
}

func (t *TableListFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	switch dir {
	case focus.DirUp:
		if t.table.AtTop() {
			return nil, false
		}
		t.table.MoveUp()
		return nil, true
	case focus.DirDown:
		if t.table.AtBottom() {
			return nil, false
		}
		t.table.MoveDown()
		return nil, true
	case focus.DirLeft, focus.DirRight:
		return nil, true // absorb; tables have no horizontal cursor
	}
	return nil, false
}

// FormFocusItem wraps a components.Form. Arrows delegate to the form's
// internal handler (field navigation, Select toggling, text-cursor motion,
// Cancel↔Submit); only when the form reports AtStart / AtEnd does the
// event escape to the previous / next chrome region.
type FormFocusItem struct {
	form     *components.Form
	activate func() tea.Cmd
}

func NewFormFocusItem(form *components.Form, activate func() tea.Cmd) *FormFocusItem {
	return &FormFocusItem{form: form, activate: activate}
}

func (f *FormFocusItem) Focus() { f.form.Focus() }
func (f *FormFocusItem) Blur()  { f.form.Blur() }

func (f *FormFocusItem) Activate() tea.Cmd {
	if f.activate == nil {
		return nil
	}
	return f.activate()
}

func (f *FormFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	switch dir {
	case focus.DirUp:
		if f.form.AtStart() {
			return nil, false
		}
		return f.form.Arrow("up"), true
	case focus.DirDown:
		if f.form.AtEnd() {
			return nil, false
		}
		return f.form.Arrow("down"), true
	case focus.DirLeft:
		return f.form.Arrow("left"), true
	case focus.DirRight:
		return f.form.Arrow("right"), true
	}
	return nil, false
}

// PublishFormFocusItem wraps the specialized PublishForm (Messages page,
// ModePublish). Arrows delegate to PublishForm.Update via synthetic
// tea.KeyMsg values; AtStart / AtEnd control escape upward to Tabs.
// The NATS client is captured at construction so Activate can call submit
// without threading the client through the focus.Manager API.
type PublishFormFocusItem struct {
	form   *PublishForm
	client *natsclient.Client
}

func NewPublishFormFocusItem(form *PublishForm, client *natsclient.Client) *PublishFormFocusItem {
	return &PublishFormFocusItem{form: form, client: client}
}

func (p *PublishFormFocusItem) Focus() {}
func (p *PublishFormFocusItem) Blur()  {}

func (p *PublishFormFocusItem) Activate() tea.Cmd {
	cmd, _ := p.form.Update(keyMsgForString("enter"), p.client)
	return cmd
}

func (p *PublishFormFocusItem) HandleArrow(dir focus.Direction) (tea.Cmd, bool) {
	if dir == focus.DirUp && p.form.AtStart() {
		return nil, false
	}
	if dir == focus.DirDown && p.form.AtEnd() {
		return nil, false
	}
	var key string
	switch dir {
	case focus.DirUp:
		key = "up"
	case focus.DirDown:
		key = "down"
	case focus.DirLeft:
		key = "left"
	case focus.DirRight:
		key = "right"
	default:
		return nil, false
	}
	cmd, _ := p.form.Update(keyMsgForString(key), p.client)
	return cmd, true
}
