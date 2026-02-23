package app

import clipboardadapter "bdtui/internal/adapters/clipboard"

func copyToClipboard(value string) error {
	return clipboardadapter.Copy(value)
}
