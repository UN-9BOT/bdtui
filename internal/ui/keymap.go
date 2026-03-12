package ui

type Keymap struct {
	Global []string
	Leader []string
	Tmux   []string
	Form   []string
}

func DefaultKeymap() Keymap {
	return Keymap{
		Global: []string{
			"j/k, ↑/↓: select issue",
			"h/l, ←/→: switch column",
			"0 / G: first/last issue in column",
			"Mouse left-click: select issue (ghost row => focus parent, board only)",
			"Enter/Space: focus details panel",
			"details: d open description, n open notes, Ctrl+X ext edit, Esc close",
			"description/notes view: j/k or ↑/↓ scroll, Ctrl+X ext edit, Esc close",
			"/: focus search",
			"f: focus search",
			"Ctrl+F: expand filters (in search)",
			"Ctrl+C / c: clear search/filters",
			"n: create issue",
			"N: create issue with parent = selected issue (closed => confirm)",
			"b: create blocked issue from selected",
			"e: edit issue",
			"Ctrl+X: open selected issue in $EDITOR and return to Edit",
			"d: delete issue",
			"x: close/reopen",
			"p/P: cycle priority forward/back",
			"s/S: cycle status forward/back",
			"z: toggle hide/show children",
			"y: copy selected issue id",
			"r: refresh",
			"g: dependency/display leader combos",
			"t: tmux leader combos",
			"?: help",
			"q: quit",
		},
		Leader: []string{
			"g B: blocker picker (Space toggle, Enter/Esc apply)",
			"g p: set parent (interactive picker)",
			"g u: jump to parent",
			"g P: clear parent",
			"g d: show dependencies",
			"g D: toggle dim override (auto → bright → dim → auto)",
			"g o: toggle sort mode",
		},
		Tmux: []string{
			"t s: send selected issue to attached tmux target",
			"t S: pick tmux target, then send selected issue",
			"t a: attach/change tmux target without sending",
			"t d: detach current tmux target",
		},
		Form: []string{
			"↑/↓: next/previous field",
			"Tab/Shift+Tab: cycle enum/select fields",
			"Ctrl+X: open fields in $EDITOR",
			"Enter: save",
			"Esc: Create closes when title is empty, Edit saves",
		},
	}
}
