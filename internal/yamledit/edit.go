// Package yamledit performs surgical, comment/anchor/order-preserving edits to
// Kubernetes manifests using kyaml. It reads documents into an RNode tree,
// mutates individual scalar nodes, and writes the same tree back — it never
// whole-doc re-marshals. Behavior is locked by byte-exact golden-file tests.
package yamledit

import (
	"bytes"
	"fmt"
	"io"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Edit is one resource change within a container, scoped to the doc selected by
// Apply's kind+name. Delete removes the field (e.g. drop a CPU limit).
type Edit struct {
	Container string
	Section   string // requests | limits
	Resource  string // cpu | memory
	Value     string
	Delete    bool
}

// podSpecPath returns the path to the PodSpec for a workload kind.
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

// Apply edits the document(s) matching kind+name and writes the result to out.
// It returns whether anything changed. When nothing changes (no matching doc,
// container absent, or values already equal), out receives the input bytes
// unchanged.
func Apply(in io.Reader, out io.Writer, kind, name string, edits []Edit) (bool, error) {
	src, err := io.ReadAll(in)
	if err != nil {
		return false, err
	}
	nodes, err := readNodes(src)
	if err != nil {
		return false, fmt.Errorf("yamledit: read: %w", err)
	}

	changed := false
	for _, doc := range nodes {
		meta, err := doc.GetMeta()
		if err != nil || meta.Kind != kind || meta.Name != name {
			continue
		}
		for _, e := range edits {
			path := append(podSpecPath(kind), "containers", "[name="+e.Container+"]")
			c, err := doc.Pipe(yaml.Lookup(path...))
			if err != nil {
				return changed, fmt.Errorf("yamledit: lookup container %q: %w", e.Container, err)
			}
			if c == nil {
				continue // container not found -> no-op
			}
			var did bool
			if e.Delete {
				did, err = deleteField(c, e)
			} else {
				did, err = setField(c, e)
			}
			if err != nil {
				return changed, err
			}
			changed = changed || did
		}
	}

	if !changed {
		_, err = out.Write(src)
		return false, err
	}
	if err := (&kio.ByteWriter{Writer: out}).Write(nodes); err != nil {
		return changed, fmt.Errorf("yamledit: write: %w", err)
	}
	return true, nil
}

// setField sets resources.<section>.<resource> = value, creating the resources
// and section maps if absent. Writing the value already present is a no-op.
func setField(container *yaml.RNode, e Edit) (bool, error) {
	if cur, _ := container.Pipe(yaml.Lookup("resources", e.Section, e.Resource)); cur != nil {
		if yaml.GetValue(cur) == e.Value {
			return false, nil
		}
	}
	if err := container.PipeE(
		yaml.LookupCreate(yaml.MappingNode, "resources", e.Section),
		yaml.SetField(e.Resource, yaml.NewScalarRNode(e.Value)),
	); err != nil {
		return false, fmt.Errorf("yamledit: set %s.%s: %w", e.Section, e.Resource, err)
	}
	return true, nil
}

// deleteField removes resources.<section>.<resource>, then cascades: an emptied
// section is removed, and emptied resources is removed too, so we never leave
// `limits: {}` noise behind.
func deleteField(container *yaml.RNode, e Edit) (bool, error) {
	section, err := container.Pipe(yaml.Lookup("resources", e.Section))
	if err != nil {
		return false, err
	}
	if section == nil {
		return false, nil
	}
	if field, err := section.Pipe(yaml.Lookup(e.Resource)); err != nil {
		return false, err
	} else if field == nil {
		return false, nil
	}
	if _, err := section.Pipe(yaml.Clear(e.Resource)); err != nil {
		return false, err
	}
	if isEmptyMap(section) {
		resources, _ := container.Pipe(yaml.Lookup("resources"))
		if resources != nil {
			if _, err := resources.Pipe(yaml.Clear(e.Section)); err != nil {
				return false, err
			}
			if isEmptyMap(resources) {
				if _, err := container.Pipe(yaml.Clear("resources")); err != nil {
					return false, err
				}
			}
		}
	}
	return true, nil
}

func isEmptyMap(n *yaml.RNode) bool {
	return n == nil || len(n.Content()) == 0
}
