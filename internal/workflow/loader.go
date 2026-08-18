package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader resolves workflow definitions from a project directory (highest
// precedence) and a global directory. Project definitions replace a same-named
// global definition wholesale; there is no field-level merge.
type Loader struct {
	GlobalDir  string
	ProjectDir string
}

// Load reads and validates a named workflow and assembles the closure of its
// directly referenced dependency files (role prompts and JSON Schemas).
// Controller-discovered project instructions (AGENTS.md/CLAUDE.md/skills) are
// not added here; callers append them to the returned Closure before building
// a snapshot.
func (l Loader) Load(ctx context.Context, name string) (*Closure, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	dir, err := l.resolveDir(name)
	if err != nil {
		return nil, err
	}

	spec, err := parseFile(filepath.Join(dir, name+".yaml"))
	if err != nil {
		return nil, err
	}

	files, err := collectDependencies(spec, dir)
	if err != nil {
		return nil, err
	}

	return &Closure{Spec: *spec, Files: files}, nil
}

func (l Loader) resolveDir(name string) (string, error) {
	path := name + ".yaml"
	if l.ProjectDir != "" {
		if fileExists(filepath.Join(l.ProjectDir, path)) {
			return l.ProjectDir, nil
		}
	}
	if l.GlobalDir != "" {
		if fileExists(filepath.Join(l.GlobalDir, path)) {
			return l.GlobalDir, nil
		}
	}
	return "", fmt.Errorf("workflow: %q not found", name)
}

func parseFile(path string) (*WorkflowSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workflow: read %s: %w", path, err)
	}
	spec, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("workflow: %s: %w", path, err)
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("workflow: %s: %w", path, err)
	}
	return spec, nil
}

// collectDependencies reads role prompt and result schema files referenced by
// the workflow, keyed by their relative path as written in the workflow.
func collectDependencies(spec *WorkflowSpec, dir string) (map[string]string, error) {
	files := map[string]string{}
	for i := range spec.Steps {
		st := &spec.Steps[i]
		if st.Type != StepAgent {
			continue
		}
		if err := addFile(files, dir, st.Role); err != nil {
			return nil, fmt.Errorf("workflow: step %q: %w", st.ID, err)
		}
		if st.ResultSchema != "" {
			if err := addFile(files, dir, st.ResultSchema); err != nil {
				return nil, fmt.Errorf("workflow: step %q: %w", st.ID, err)
			}
		}
	}
	return files, nil
}

func addFile(files map[string]string, dir, rel string) error {
	if err := validateRelPath(rel); err != nil {
		return err
	}
	if _, ok := files[rel]; ok {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return fmt.Errorf("read dependency %q: %w", rel, err)
	}
	files[rel] = string(data)
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("workflow: name is required")
	}
	if err := validateRelPath(name); err != nil {
		return err
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("workflow: invalid name %q: must be a bare name without path separators", name)
	}
	return nil
}

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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
