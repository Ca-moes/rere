// Package fieldmap resolves the YAML path(s) where a workload's container
// resources live. Tier 1 infers the path for raw workloads (Deployment /
// StatefulSet / DaemonSet) with zero config; later tiers (operator CRs, Helm
// values) slot in behind the same FieldMapper interface without touching
// callers. Resolve returns paths and values only — it never mutates.
package fieldmap

import "sigs.k8s.io/kustomize/kyaml/yaml"

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
	// Resolve returns one ResolvedEdit per wanted field, after verifying the
	// container exists. It errors if the container is absent (no phantom
	// creation).
	Resolve(root *yaml.RNode, t Target, want map[ResourceField]string) ([]ResolvedEdit, error)
}
