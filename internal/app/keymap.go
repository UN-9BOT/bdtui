package app

import ui "bdtui/internal/ui"

type Keymap = ui.Keymap

func defaultKeymap() Keymap {
	return ui.DefaultKeymap()
}
