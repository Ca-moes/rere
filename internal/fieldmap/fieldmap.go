// Package fieldmap resolves the YAML path(s) where a workload's container
// resources live. Tier 1 infers the path for raw workloads (Deployment /
// StatefulSet / DaemonSet) with zero config; later tiers (operator CRs, Helm
// values) slot in behind the same FieldMapper interface without touching
// callers. Resolve returns paths and values only — it never mutates.
package fieldmap

import (
	"sort"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// ResourceField names one resource cell: a section (requests|limits) and a
// resource (cpu|memory).
type ResourceField struct {
	Section string
	Name    string
}

// ResolvedEdit is a concrete instruction for the editor: set Value at Path.
type ResolvedEdit struct {
	Field ResourceField
	Path  []string
	Value string
}

// Target identifies the container whose resources to resolve.
type Target struct {
	Kind          string
	Container     string
	InitContainer bool
}

// FieldMapper resolves resource fields for the workloads it Supports.
type FieldMapper interface {
	// Supports reports whether this mapper handles the given manifest.
	Supports(root *yaml.RNode) bool
	// ResolvePath returns the absolute YAML path of a single resource cell,
	// after verifying the addressed container/subtree exists. It errors if it is
	// absent (no phantom creation). Single-cell and value-free so deletes (which
	// carry no value) are first-class.
	ResolvePath(root *yaml.RNode, t Target, f ResourceField) ([]string, error)
	// Resolve returns one ResolvedEdit per wanted field, in a deterministic
	// (section, name) order. It errors if the container is absent.
	Resolve(root *yaml.RNode, t Target, want map[ResourceField]string) ([]ResolvedEdit, error)
}

// sortEdits orders edits deterministically by (section, name) — limits before
// requests, cpu before memory — so output is stable across runs. Shared by all
// tiers.
func sortEdits(edits []ResolvedEdit) {
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].Field.Section != edits[j].Field.Section {
			return edits[i].Field.Section < edits[j].Field.Section
		}
		return edits[i].Field.Name < edits[j].Field.Name
	})
}
