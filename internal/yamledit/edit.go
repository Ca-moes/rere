// Package yamledit performs surgical, comment/anchor/order-preserving edits to
// Kubernetes manifests using kyaml. It reads documents into an RNode tree,
// mutates individual scalar nodes, and writes the same tree back — it never
// whole-doc re-marshals. Behavior is locked by byte-exact golden-file tests.
//
// Edits are addressed by absolute path (see ApplyPaths); the path is resolved by
// a fieldmap.FieldMapper, so this package knows nothing about workload kinds.
package yamledit

import (
	"bytes"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Edit is one resource change within a container's resources block. It is the
// shape the policy engine emits; the run loop resolves each Edit's
// (Section, Resource) to an absolute path and applies it via ApplyPaths. Delete
// removes the field (e.g. drop a CPU limit).
type Edit struct {
	Container string
	Section   string // requests | limits
	Resource  string // cpu | memory
	Value     string
	Delete    bool
}

// podSpecPath returns the path to the PodSpec for a workload kind. Used by the
// tier-1 ReadCurrent shim.
func podSpecPath(kind string) []string {
	if kind == "CronJob" {
		return []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	return []string{"spec", "template", "spec"}
}

func readNodes(src []byte) ([]*yaml.RNode, error) {
	// PreserveSeqIndent keeps list indentation byte-identical, which requires
	// retaining reader annotations; ByteWriter strips the internal ones before
	// emitting, so they never leak into the output.
	return (&kio.ByteReader{
		Reader:            bytes.NewReader(src),
		PreserveSeqIndent: true,
	}).Read()
}

func isEmptyMap(n *yaml.RNode) bool {
	return n == nil || len(n.Content()) == 0
}
