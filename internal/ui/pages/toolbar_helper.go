package pages

import (
	tea "github.com/charmbracelet/bubbletea"
)

// toolbarKeyCmd returns a Cmd that emits a tea.KeyMsg equivalent to the
// keyboard shortcut for the given toolbar action ID. Using a synthesized
// KeyMsg guarantees clicks and keypresses run through the same code paths
// in Update — no separate mouse dispatch branch anywhere.
//
// mapFn maps an action ID to its shortcut key. Return "" for unknown IDs.
// Special key names ("esc", "enter", "tab", "backspace") map to their
// tea.KeyType constants; anything else is treated as printable runes.
func toolbarKeyCmd(actionID string, mapFn func(string) string) tea.Cmd {
	key := mapFn(actionID)
	if key == "" {
		return nil
	}
	km := keyMsgForString(key)
	return func() tea.Msg { return km }
}

func keyMsgForString(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
