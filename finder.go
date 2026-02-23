package bdtui

import (
	"fmt"
	"os"
	"path/filepath"
)

func findBeadsDir(explicit string) (beadsDir string, repoDir string, err error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", "", err
		}
		if !st.IsDir() {
			return "", "", fmt.Errorf("beads-dir is not a directory: %s", abs)
		}
		if filepath.Base(abs) != ".beads" {
			return "", "", fmt.Errorf("beads-dir must point to .beads directory: %s", abs)
		}
		return abs, filepath.Dir(abs), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	current := cwd
	for {
		candidate := filepath.Join(current, ".beads")
		st, err := os.Stat(candidate)
		if err == nil && st.IsDir() {
			return candidate, current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", "", fmt.Errorf("no .beads directory found from %s up to root", cwd)
}
