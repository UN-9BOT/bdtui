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
			"Enter/Space: детали задачи",
			"/: поиск",
			"f: фильтры",
			"c: сбросить поиск/фильтры",
			"n: создать задачу",
			"e: редактировать задачу",
			"d: удалить задачу",
			"x: close/reopen",
			"p: cycle priority",
			"s: cycle status",
			"a: quick assignee",
			"Shift+L: quick labels",
			"r: refresh",
			"g: leader combos",
			"?: помощь",
			"q/Ctrl+C: выход",
		},
		Leader: []string{
			"g b: добавить blocker",
			"g B: удалить blocker",
			"g p: назначить parent",
			"g P: снять parent",
			"g d: показать зависимости",
		},
		Form: []string{
			"Tab/Shift+Tab: следующее/предыдущее поле",
			"↑/↓: смена enum-полей",
			"Ctrl+X: открыть поля в $EDITOR",
			"Enter: сохранить",
			"Esc: отмена",
		},
	}
}
