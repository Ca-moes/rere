package fieldmap

import (
	"fmt"

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

// ResolvePath verifies the container exists under spec.template.spec and returns
// the absolute path of one resource cell. It errors if the container is absent.
func (Tier1) ResolvePath(root *yaml.RNode, t Target, f ResourceField) ([]string, error) {
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

	return []string{"spec", "template", "spec", containersField, elem, "resources", f.Section, f.Name}, nil
}

// Resolve returns a ResolvedEdit per wanted field, in a deterministic
// (section, name) order. It errors if the container is absent (no phantom
// creation).
func (m Tier1) Resolve(root *yaml.RNode, t Target, want map[ResourceField]string) ([]ResolvedEdit, error) {
	return resolveWant(func(f ResourceField) ([]string, error) {
		return m.ResolvePath(root, t, f)
	}, want)
}
