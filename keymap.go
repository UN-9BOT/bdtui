package main

type Keymap struct {
	Global []string
	Leader []string
	Form   []string
}

func defaultKeymap() Keymap {
	return Keymap{
		Global: []string{
			"j/k, ↑/↓: select issue",
			"h/l, ←/→: switch column",
			"0 / G: first/last issue in column",
			"Enter/Space: focus details panel",
			"details: j/k or ↑/↓ scroll, Esc close",
			"/: search",
			"f: filters",
			"c: clear search/filters",
			"n: create issue",
			"N: create issue with parent = selected issue",
			"e: edit issue",
			"Ctrl+X: open selected issue in $EDITOR and return to Edit",
			"d: delete issue",
			"x: close/reopen",
			"p/P: cycle priority forward/back",
			"s/S: cycle status forward/back",
			"a: quick assignee",
			"y: copy selected issue id",
			"Y: paste selected issue id into tmux pane (picker marks pane)",
			"Shift+L: quick labels",
			"r: refresh",
			"g: leader combos",
			"?: help",
			"q: quit",
		},
		Leader: []string{
			"g b: add blocker",
			"g B: remove blocker",
			"g p: set parent (interactive picker)",
			"g P: clear parent",
			"g d: show dependencies",
			"g o: toggle sort mode",
		},
		Form: []string{
			"Tab/Shift+Tab: next/previous field",
			"↑/↓: cycle enum fields",
			"Ctrl+X: open fields in $EDITOR",
			"Enter: save",
			"Esc: Create closes when title is empty, Edit saves",
		},
	}
}
