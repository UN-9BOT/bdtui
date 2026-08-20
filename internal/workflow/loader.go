package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ListEntry identifies one workflow discovered by a Loader. Origin records
// which root the entry came from; when both roots define a workflow with the
// same name, the project root wins and the global root is suppressed so callers
// can render a flat, deduplicated list.
type ListEntry struct {
	Name   string
	Origin string // "project" or "global"
}

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

// List returns the deduplicated set of workflow names visible to this Loader.
// The project root wins over the global root on name collision. Results are
// sorted by name. A root that does not exist is treated as empty (no error).
// Files that are not valid YAML are skipped so a single corrupt file does not
// hide every other workflow.
func (l Loader) List(_ context.Context) ([]ListEntry, error) {
	project := l.scanDir(l.Project)
	global := l.scanDir(l.Global)

	merged := make(map[string]ListEntry, len(project)+len(global))
	for _, e := range project {
		merged[e.Name] = e
	}
	for _, e := range global {
		if _, ok := merged[e.Name]; ok {
			continue
		}
		merged[e.Name] = e
	}

	out := make([]ListEntry, 0, len(merged))
	for _, e := range merged {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (l Loader) scanDir(root string) []ListEntry {
	if root == "" {
		return nil
	}
	dir := filepath.Join(root, "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]ListEntry, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := strings.TrimSuffix(ent.Name(), ".yaml")
		if name == ent.Name() || name == "" {
			continue
		}
		if err := validateID(name); err != nil {
			continue
		}
		origin := "global"
		if root == l.Project {
			origin = "project"
		}
		out = append(out, ListEntry{Name: name, Origin: origin})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Load resolves a named workflow and assembles its bundle: the workflow, the
// resolved role contracts it references, and the raw contents of referenced
// prompt and schema files. Controller-discovered project instructions
// (AGENTS.md/CLAUDE.md/skills) are not added here; callers append them to the
// returned Bundle.Files before building a snapshot.
func (l Loader) Load(ctx context.Context, name string) (*Bundle, error) {
	if err := validateID(name); err != nil {
		return nil, fmt.Errorf("workflow: name: %w", err)
	}

	workflowDir, err := l.resolveWorkflowDir(name)
	if err != nil {
		return nil, err
	}

	spec, source, err := parseWorkflowFile(filepath.Join(workflowDir, "workflows", name+".yaml"))
	if err != nil {
		return nil, err
	}

	roles, files, err := l.resolveRoles(spec)
	if err != nil {
		return nil, err
	}

	bundle := &Bundle{Spec: *spec, Roles: roles, Files: files, WorkflowSource: source}
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
		if role.ID != st.Role {
			return nil, nil, fmt.Errorf("workflow: role file for %q declares id %q", st.Role, role.ID)
		}
		roles[st.Role] = *role

		if err := addDependency(files, rolePromptKey(st.Role), dir, role.Prompt); err != nil {
			return nil, nil, fmt.Errorf("workflow: role %q: %w", st.Role, err)
		}
		if role.ResultSchema != "" {
			if err := addDependency(files, roleSchemaKey(st.Role), dir, role.ResultSchema); err != nil {
				return nil, nil, fmt.Errorf("workflow: role %q: %w", st.Role, err)
			}
		}
	}
	return roles, files, nil
}

// rolePromptKey and roleSchemaKey namespace dependency files by role id so a
// prompt/schema with the same relative path from a global and a project root
// cannot collide in the snapshot closure.
func rolePromptKey(roleID string) string { return "roles/" + roleID + "/prompt" }
func roleSchemaKey(roleID string) string { return "roles/" + roleID + "/schema" }

func parseWorkflowFile(path string) (*WorkflowSpec, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("workflow: read %s: %w", path, err)
	}
	spec, err := Parse(data)
	if err != nil {
		return nil, "", fmt.Errorf("workflow: %s: %w", path, err)
	}
	if err := spec.Validate(); err != nil {
		return nil, "", fmt.Errorf("workflow: %s: %w", path, err)
	}
	return spec, string(data), nil
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

func addDependency(files map[string]string, key, dir, rel string) error {
	if err := validateRelPath(rel); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return fmt.Errorf("read dependency %q: %w", rel, err)
	}
	files[key] = string(data)
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
