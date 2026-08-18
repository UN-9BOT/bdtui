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
	tag := normalizeYAMLTag(n.Tag)
	if n.Kind == yaml.ScalarNode && tag == "!!merge" {
		return fmt.Errorf("merge keys are not supported")
	}
	if tag != "" && !standardYAMLTags[tag] {
		return fmt.Errorf("custom tags are not supported (tag %q)", n.Tag)
	}
	for _, c := range n.Content {
		if err := validateYAMLNode(c); err != nil {
			return err
		}
	}
	return nil
}

// normalizeYAMLTag converts a long-form yaml.org,2002 tag to its short "!!"
// form so that merge-key and custom-tag checks share one canonical allowlist.
func normalizeYAMLTag(tag string) string {
	const prefix = "tag:yaml.org,2002:"
	if strings.HasPrefix(tag, prefix) {
		return "!!" + strings.TrimPrefix(tag, prefix)
	}
	return tag
}
