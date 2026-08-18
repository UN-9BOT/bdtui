package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader resolves workflow and role definitions from a project definitions
// root (highest precedence) and a global definitions root. Workflows and roles
// override independently: a project workflow with a given name replaces the
// global workflow with the same name, and a project role with a given id
// replaces the global role with the same id. A project workflow may still
// reference a global role when no project role override exists.
//
// Layout under each root:
//
//	<root>/workflows/<name>.yaml
//	<root>/roles/<id>.yaml
//
// Role prompt and result_schema paths are relative to the role's root.
type Loader struct {
	Global  string
	Project string
}

// Load resolves a named workflow and assembles its bundle: the workflow, the
// resolved role contracts it references, and the raw contents of referenced
// prompt and schema files. Controller-discovered project instructions
// (AGENTS.md/CLAUDE.md/skills) are not added here; callers append them to the
// returned Bundle.Files before building a snapshot.
func (l Loader) Load(ctx context.Context, name string) (*Bundle, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	workflowDir, err := l.resolveWorkflowDir(name)
	if err != nil {
		return nil, err
	}

	spec, err := parseWorkflowFile(filepath.Join(workflowDir, "workflows", name+".yaml"))
	if err != nil {
		return nil, err
	}

	roles, files, err := l.resolveRoles(spec)
	if err != nil {
		return nil, err
	}

	bundle := &Bundle{Spec: *spec, Roles: roles, Files: files}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (l Loader) resolveWorkflowDir(name string) (string, error) {
	rel := filepath.Join("workflows", name+".yaml")
	if l.Project != "" && fileExists(filepath.Join(l.Project, rel)) {
		return l.Project, nil
	}
	if l.Global != "" && fileExists(filepath.Join(l.Global, rel)) {
		return l.Global, nil
	}
	return "", fmt.Errorf("workflow: %q not found", name)
}

func (l Loader) resolveRoleDir(id string) (string, error) {
	rel := filepath.Join("roles", id+".yaml")
	if l.Project != "" && fileExists(filepath.Join(l.Project, rel)) {
		return l.Project, nil
	}
	if l.Global != "" && fileExists(filepath.Join(l.Global, rel)) {
		return l.Global, nil
	}
	return "", fmt.Errorf("workflow: role %q not found", id)
}

func (l Loader) resolveRoles(spec *WorkflowSpec) (map[string]RoleContract, map[string]string, error) {
	roles := map[string]RoleContract{}
	files := map[string]string{}

	for i := range spec.Steps {
		st := &spec.Steps[i]
		if st.Type != StepAgent {
			continue
		}
		if _, ok := roles[st.Role]; ok {
			continue
		}

		dir, err := l.resolveRoleDir(st.Role)
		if err != nil {
			return nil, nil, err
		}
		role, err := parseRoleFile(filepath.Join(dir, "roles", st.Role+".yaml"))
		if err != nil {
			return nil, nil, err
		}
		roles[st.Role] = *role

		if err := addFile(files, dir, role.Prompt); err != nil {
			return nil, nil, fmt.Errorf("workflow: role %q: %w", st.Role, err)
		}
		if role.ResultSchema != "" {
			if err := addFile(files, dir, role.ResultSchema); err != nil {
				return nil, nil, fmt.Errorf("workflow: role %q: %w", st.Role, err)
			}
		}
	}
	return roles, files, nil
}

func parseWorkflowFile(path string) (*WorkflowSpec, error) {
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

func parseRoleFile(path string) (*RoleContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workflow: read %s: %w", path, err)
	}
	role, err := ParseRole(data)
	if err != nil {
		return nil, fmt.Errorf("workflow: %s: %w", path, err)
	}
	if err := role.Validate(); err != nil {
		return nil, fmt.Errorf("workflow: %s: %w", path, err)
	}
	return role, nil
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
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("workflow: invalid name %q: must be a bare name without path separators", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("workflow: invalid name %q", name)
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
