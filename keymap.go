package main

type Keymap struct {
	Global []string
	Leader []string
	Form   []string
}

func defaultKeymap() Keymap {
	return Keymap{
		Global: []string{
			"j/k, ↑/↓: выбор карточки",
			"h/l, ←/→: смена колонки",
			"0 / G: начало/конец колонки",
			"Enter/Space: расширить нижний блок",
			"/: поиск",
			"f: фильтры",
			"c: сбросить поиск/фильтры",
			"n: создать задачу",
			"e: редактировать задачу",
			"Ctrl+X: открыть выбранную задачу в $EDITOR и вернуться в Edit",
			"d: удалить задачу",
			"x: close/reopen",
			"p: cycle priority",
			"s: cycle status",
			"a: quick assignee",
			"y: copy selected issue id",
			"Y: paste selected issue id into tmux pane",
			"Shift+L: quick labels",
			"r: refresh",
			"g: leader combos",
			"?: помощь",
			"q: выход",
		},
		Leader: []string{
			"g b: добавить blocker",
			"g B: удалить blocker",
			"g p: назначить parent (интерактивный выбор)",
			"g P: снять parent",
			"g d: показать зависимости",
		},
		Form: []string{
			"Tab/Shift+Tab: следующее/предыдущее поле",
			"↑/↓: смена enum-полей",
			"Ctrl+X: открыть поля в $EDITOR",
			"Enter/Esc: сохранить",
		},
	}
}
