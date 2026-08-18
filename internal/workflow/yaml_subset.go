package workflow

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// standardYAMLTags is the allowlist of short-form YAML core-schema tags that
// this package accepts. Anything else with a non-empty tag is a custom tag and
// is rejected before strict field decoding.
var standardYAMLTags = map[string]bool{
	"!!str":       true,
	"!!seq":       true,
	"!!map":       true,
	"!!int":       true,
	"!!float":     true,
	"!!bool":      true,
	"!!null":      true,
	"!!binary":    true,
	"!!timestamp": true,
	"!!merge":     true,
	"!!omap":      true,
	"!!pairs":     true,
	"!!set":       true,
	"!!value":     true,
	"!!yaml":      true,
}

// validateYAMLSubset preflights a YAML document against the deliberately small
// syntax subset this package supports: no anchors, no aliases, no merge keys,
// and no custom tags. It runs before strict field decoding so forbidden syntax
// is rejected even when it would otherwise decode into a struct.
func validateYAMLSubset(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	return validateYAMLNode(&root)
}

// validateYAMLNode recursively rejects the YAML syntax constructs that are not
// part of the supported subset: anchors, aliases, merge keys, and custom tags.
func validateYAMLNode(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Anchor != "" {
		return fmt.Errorf("anchors are not supported (anchor %q)", n.Anchor)
	}
	if n.Kind == yaml.AliasNode {
		return fmt.Errorf("aliases are not supported (alias %q)", n.Value)
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!merge" {
		return fmt.Errorf("merge keys are not supported")
	}
	if isCustomTag(n.Tag) {
		return fmt.Errorf("custom tags are not supported (tag %q)", n.Tag)
	}
	for _, c := range n.Content {
		if err := validateYAMLNode(c); err != nil {
			return err
		}
	}
	return nil
}

// isCustomTag reports whether tag is a YAML custom tag rather than a standard
// core-schema tag. An empty tag means the node was untagged and resolved by the
// schema, so it is not custom.
func isCustomTag(tag string) bool {
	if tag == "" {
		return false
	}
	// Long-form tags are standard only in the yaml.org,2002 namespace.
	if strings.HasPrefix(tag, "tag:yaml.org,2002:") {
		return false
	}
	// Short-form standard tags use the "!!" handle and are allowlisted.
	return !standardYAMLTags[tag]
}
