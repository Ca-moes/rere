package fieldmap

import (
	"fmt"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Tier2 handles operator CRs whose resources live at a config-driven path. Maps
// is the merged registry (built-ins overlaid by user config); construct it with
// MergedMaps so the built-in CNPG/OTel maps are always present.
type Tier2 struct {
	Maps MapConfig
}

// groupKind extracts the apiVersion group and kind from a manifest. The core
// group ("v1") yields an empty group.
func groupKind(root *yaml.RNode) (group, kind string) {
	meta, err := root.GetMeta()
	if err != nil {
		return "", ""
	}
	if g, _, found := strings.Cut(meta.APIVersion, "/"); found {
		group = g
	}
	return group, meta.Kind
}

// Supports reports whether a CRMap matches the manifest's (group, kind).
func (m Tier2) Supports(root *yaml.RNode) bool {
	group, kind := groupKind(root)
	return findCRMap(m.Maps, group, kind) != nil
}

// ResolvePath resolves one resource cell within the CR's resources block, after
// verifying that block's parent exists (the resources map itself may be
// created, but never its parent — no phantom subtrees).
func (m Tier2) ResolvePath(root *yaml.RNode, t Target, f ResourceField) ([]string, error) {
	group, kind := groupKind(root)
	cm := findCRMap(m.Maps, group, kind)
	if cm == nil {
		return nil, fmt.Errorf("fieldmap: no tier-2 map for %s/%s", group, kind)
	}
	base, err := basePath(cm, t)
	if err != nil {
		return nil, err
	}
	if parent := base[:len(base)-1]; len(parent) > 0 {
		n, err := root.Pipe(yaml.Lookup(parent...))
		if err != nil {
			return nil, fmt.Errorf("fieldmap: lookup %v in %s/%s: %w", parent, group, kind, err)
		}
		if n == nil {
			return nil, fmt.Errorf("fieldmap: %s/%s: resources parent %v not found", group, kind, parent)
		}
	}
	return append(append([]string{}, base...), f.Section, f.Name), nil
}

// Resolve returns a ResolvedEdit per wanted field in deterministic order.
func (m Tier2) Resolve(root *yaml.RNode, t Target, want map[ResourceField]string) ([]ResolvedEdit, error) {
	return resolveWant(func(f ResourceField) ([]string, error) {
		return m.ResolvePath(root, t, f)
	}, want)
}

// basePath is the path to a CR's resources mapping. For multi-component CRs the
// Target.Container is the component name (TranslateTarget already mapped the
// reported container to it).
func basePath(cm *CRMap, t Target) ([]string, error) {
	if len(cm.Components) == 0 {
		return cm.ResourcePath, nil
	}
	for _, comp := range cm.Components {
		if comp.Name == t.Container {
			return comp.Path, nil
		}
	}
	return nil, fmt.Errorf("fieldmap: %s/%s: no component named %q", cm.Group, cm.Kind, t.Container)
}
