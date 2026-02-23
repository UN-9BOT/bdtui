package app

import beadsadapter "bdtui/internal/adapters/beads"

func findBeadsDir(explicit string) (beadsDir string, repoDir string, err error) {
	return beadsadapter.FindBeadsDir(explicit)
}
