package yamledit

import (
	"fmt"
	"io"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// PathEdit is a resolved, path-addressed scalar change produced by a
// FieldMapper: set Value at Path (or remove it when Delete is set). Path is
// absolute from the document root, e.g.
// spec.template.spec.containers.[name=web].resources.requests.cpu — so the
// editor needs no knowledge of workload kinds, PodSpecs, or containers.
type PathEdit struct {
	Path   []string
	Value  string
	Delete bool
}

// ApplyPaths edits the document(s) matching kind+name and writes the result to
// out. It returns whether anything changed. When nothing changes (no matching
// doc, an addressed sequence element absent, or values already equal), out
// receives the input bytes unchanged.
//
// A path is split at its last sequence-element selector (e.g. [name=web]): the
// prefix up to and including that selector must already exist (a missing list
// entry is a no-op, never fabricated), while the trailing map keys are created
// as needed. This reproduces the legacy editor's "container not found -> no-op,
// otherwise create resources/section maps" contract for tier-1 paths, and
// generalizes to selector-free CR paths (which operate from the document root).
func ApplyPaths(in io.Reader, out io.Writer, kind, name string, edits []PathEdit) (bool, error) {
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
			prefix, suffix := splitAtLastSelector(e.Path)
			node := doc
			if len(prefix) > 0 {
				node, err = doc.Pipe(yaml.Lookup(prefix...))
				if err != nil {
					return changed, fmt.Errorf("yamledit: lookup %v: %w", prefix, err)
				}
				if node == nil {
					continue // addressed sequence element absent -> no-op
				}
			}
			var did bool
			if e.Delete {
				did, err = deleteAt(node, suffix)
			} else {
				did, err = setAt(node, suffix, e.Value)
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

// splitAtLastSelector divides path into the prefix up to and including the last
// sequence-element selector (a segment like "[name=web]") and the trailing map
// keys. With no selector the whole path is map keys and prefix is empty.
func splitAtLastSelector(path []string) (prefix, suffix []string) {
	idx := -1
	for i, seg := range path {
		if strings.HasPrefix(seg, "[") {
			idx = i
		}
	}
	if idx == -1 {
		return nil, path
	}
	return path[:idx+1], path[idx+1:]
}

// lookupRel resolves parts relative to node, returning node itself for an empty
// path.
func lookupRel(node *yaml.RNode, parts []string) (*yaml.RNode, error) {
	if len(parts) == 0 {
		return node, nil
	}
	return node.Pipe(yaml.Lookup(parts...))
}

// setAt sets the scalar at suffix (relative to node) to value, creating
// intermediate maps as needed. Writing the value already present is a no-op.
func setAt(node *yaml.RNode, suffix []string, value string) (bool, error) {
	if len(suffix) == 0 {
		return false, fmt.Errorf("yamledit: empty edit path")
	}
	if cur, _ := lookupRel(node, suffix); cur != nil && yaml.GetValue(cur) == value {
		return false, nil
	}
	parent, leaf := suffix[:len(suffix)-1], suffix[len(suffix)-1]
	if err := node.PipeE(
		yaml.LookupCreate(yaml.MappingNode, parent...),
		yaml.SetField(leaf, yaml.NewScalarRNode(value)),
	); err != nil {
		return false, fmt.Errorf("yamledit: set %v: %w", suffix, err)
	}
	return true, nil
}

// deleteAt removes the scalar at suffix (relative to node), then cascades:
// emptied ancestor maps are removed too, halting at the first non-empty ancestor
// so we never leave `limits: {}` noise nor remove node itself.
func deleteAt(node *yaml.RNode, suffix []string) (bool, error) {
	if len(suffix) == 0 {
		return false, fmt.Errorf("yamledit: empty edit path")
	}
	m := len(suffix)
	parent, err := lookupRel(node, suffix[:m-1])
	if err != nil {
		return false, err
	}
	if parent == nil {
		return false, nil
	}
	if field, err := parent.Pipe(yaml.Lookup(suffix[m-1])); err != nil {
		return false, err
	} else if field == nil {
		return false, nil
	}
	if _, err := parent.Pipe(yaml.Clear(suffix[m-1])); err != nil {
		return false, err
	}
	// Walk up: if the map we just edited is now empty, clear it from its parent.
	for d := m - 1; d >= 1; d-- {
		cur, err := lookupRel(node, suffix[:d])
		if err != nil {
			return false, err
		}
		if cur == nil || !isEmptyMap(cur) {
			break
		}
		gp, err := lookupRel(node, suffix[:d-1])
		if err != nil {
			return false, err
		}
		if gp != nil {
			if _, err := gp.Pipe(yaml.Clear(suffix[d-1])); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}
