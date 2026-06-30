package fieldmap

import (
	"fmt"
	"sort"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Tier1 handles raw workloads whose pod template lives at spec.template.spec.
type Tier1 struct{}

var tier1Kinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
}

// Supports reports whether root is a raw workload kind Tier1 understands.
func (Tier1) Supports(root *yaml.RNode) bool {
	meta, err := root.GetMeta()
	if err != nil {
		return false
	}
	return Tier1Supports(meta.Kind)
}

// Tier1Supports reports whether a workload kind is handled by Tier1. Exposed so
// callers can gate on kind without parsing the manifest.
func Tier1Supports(kind string) bool {
	return tier1Kinds[kind]
}

// Resolve verifies the container exists under spec.template.spec and returns a
// ResolvedEdit per wanted field, in a deterministic (section, name) order.
func (Tier1) Resolve(root *yaml.RNode, t Target, want map[ResourceField]string) ([]ResolvedEdit, error) {
	containersField := "containers"
	if t.InitContainer {
		containersField = "initContainers"
	}

	elem := "[name=" + t.Container + "]"
	c, err := root.Pipe(yaml.Lookup("spec", "template", "spec", containersField, elem))
	if err != nil {
		return nil, fmt.Errorf("fieldmap: lookup container %q: %w", t.Container, err)
	}
	if c == nil {
		return nil, fmt.Errorf("fieldmap: container %q not found in %s %s", t.Container, t.Kind, containersField)
	}

	base := []string{"spec", "template", "spec", containersField, elem, "resources"}
	edits := make([]ResolvedEdit, 0, len(want))
	for f, v := range want {
		path := append(append([]string{}, base...), f.Section, f.Name)
		edits = append(edits, ResolvedEdit{Field: f, Path: path, Value: v})
	}
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].Field.Section != edits[j].Field.Section {
			return edits[i].Field.Section < edits[j].Field.Section
		}
		return edits[i].Field.Name < edits[j].Field.Name
	})
	return edits, nil
}
