package workflow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateID checks a workflow name or role id. It must be a bare identifier:
// no path separators and no traversal components.
func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("id %q must not contain path separators", id)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("id %q is not allowed", id)
	}
	return nil
}

// validateRelPath checks that a file reference is a relative path that cannot
// escape its root.
func validateRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("path %q must be relative", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("path %q must not contain '..'", p)
		}
	}
	return nil
}
