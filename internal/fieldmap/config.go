package fieldmap

import (
	"fmt"
	"regexp"
)

// MapConfig is the tier-2 field-map registry: built-in maps overlaid by
// user-provided maps. It tells the tier-2 mapper where an operator CR keeps its
// container resources, and how to translate a recommender's generated-workload
// identity back to the owning CR.
type MapConfig struct {
	Maps []CRMap `json:"maps"`
}

// CRMap maps one operator CR (identified by apiVersion group + kind) to where
// its resources live. Single-component CRs use ResourcePath; CRs with several
// resource-bearing components use Components for name-aware selection. Exactly
// one of the two must be set.
type CRMap struct {
	Group        string      `json:"group"` // apiVersion group, e.g. "postgresql.cnpg.io" (version-agnostic)
	Kind         string      `json:"kind"`  // e.g. "Cluster"
	ResourcePath []string    `json:"resourcePath,omitempty"`
	Components   []Component `json:"components,omitempty"`
	Match        MatchRule   `json:"match,omitempty"`

	// nameRE is the compiled Match.NamePattern, cached by MergedMaps so
	// translation does not recompile it per target. Unexported: not serialized.
	nameRE *regexp.Regexp
}

// Component is one named resource-bearing subtree within a multi-component CR.
type Component struct {
	Name string   `json:"name"`
	Path []string `json:"path"`
}

// MatchRule translates a recommender's reported (kind, name, container) — which
// names the operator-generated workload, not the CR — back to this CR and one of
// its components.
type MatchRule struct {
	// WorkloadKind is what the recommender reports for this CR's pods (e.g.
	// "Deployment", "Pod"). Empty matches any kind.
	WorkloadKind string `json:"workloadKind,omitempty"`
	// NameSuffix is trimmed from the workload name to recover the CR name (e.g.
	// "-collector": otel-collector -> otel).
	NameSuffix string `json:"nameSuffix,omitempty"`
	// NamePattern is an alternative to NameSuffix for numeric-instance names: a
	// regexp whose first capture group is the CR name (e.g. "^(.*)-[0-9]+$":
	// mycluster-1 -> mycluster).
	NamePattern string `json:"namePattern,omitempty"`
	// ContainerToComponent maps a reported container name to a Component.Name.
	// Empty (or a "") value selects the single-component ResourcePath.
	ContainerToComponent map[string]string `json:"containerToComponent,omitempty"`
}

// Validate checks every map is well-formed and that (group, kind) is unique.
func (c MapConfig) Validate() error {
	seen := map[string]bool{}
	for i, m := range c.Maps {
		if m.Group == "" || m.Kind == "" {
			return fmt.Errorf("fieldMaps.maps[%d]: group and kind are required", i)
		}
		hasPath := len(m.ResourcePath) > 0
		hasComp := len(m.Components) > 0
		if hasPath == hasComp {
			return fmt.Errorf("fieldMaps.maps[%d] (%s/%s): set exactly one of resourcePath or components", i, m.Group, m.Kind)
		}
		for j, comp := range m.Components {
			if comp.Name == "" || len(comp.Path) == 0 {
				return fmt.Errorf("fieldMaps.maps[%d].components[%d]: name and path are required", i, j)
			}
		}
		if m.Match.NamePattern != "" {
			re, err := regexp.Compile(m.Match.NamePattern)
			if err != nil {
				return fmt.Errorf("fieldMaps.maps[%d] (%s/%s): invalid namePattern: %w", i, m.Group, m.Kind, err)
			}
			if re.NumSubexp() < 1 {
				return fmt.Errorf("fieldMaps.maps[%d] (%s/%s): namePattern must have a capture group for the CR name", i, m.Group, m.Kind)
			}
		}
		key := m.Group + "/" + m.Kind
		if seen[key] {
			return fmt.Errorf("fieldMaps: duplicate map for %s", key)
		}
		seen[key] = true
	}
	return nil
}

// findCRMap returns the map for a (group, kind), or nil.
func findCRMap(c MapConfig, group, kind string) *CRMap {
	for i := range c.Maps {
		if c.Maps[i].Group == group && c.Maps[i].Kind == kind {
			return &c.Maps[i]
		}
	}
	return nil
}

// MergedMaps overlays user maps on the built-ins. User maps come FIRST so they
// take precedence in translation (TranslateTarget returns on first match): a
// user operator whose generated-workload names collide with a built-in
// heuristic (the `-collector` suffix, `-<digits>` pods) resolves to the user's
// CR, not the built-in's. A built-in is dropped when a user map shares its
// (group, kind). Built-ins live in code so a config-less binary still
// right-sizes the common operators.
func MergedMaps(user MapConfig) MapConfig {
	out := MapConfig{Maps: append([]CRMap{}, user.Maps...)}
	for _, bm := range BuiltinMaps().Maps {
		if findCRMap(out, bm.Group, bm.Kind) == nil {
			out.Maps = append(out.Maps, bm)
		}
	}
	// Compile NamePatterns once here so translation reuses them. Patterns are
	// validated before this (config.Validate / known-good built-ins); a stray
	// bad one leaves nameRE nil and translation falls back to a lazy compile.
	for i := range out.Maps {
		if p := out.Maps[i].Match.NamePattern; p != "" {
			out.Maps[i].nameRE, _ = regexp.Compile(p)
		}
	}
	return out
}

// BuiltinMaps are the operator CRs rere maps out of the box. Both store a
// standard core/v1 ResourceRequirements at spec.resources.
//
// NOTE: the Match rules below are best-effort and not yet verified against a
// real recommender run (see #7 / the #28 follow-up). Capture a real `krr -f
// json` against a cluster running these operators and confirm the reported
// (kind, name, container) before relying on the built-in defaults.
func BuiltinMaps() MapConfig {
	return MapConfig{Maps: []CRMap{
		{
			Group:        "postgresql.cnpg.io",
			Kind:         "Cluster",
			ResourcePath: []string{"spec", "resources"},
			Match: MatchRule{
				WorkloadKind: "Pod", // TODO verify: CNPG manages bare Pods, one per instance
				NamePattern:  `^(.*)-[0-9]+$`,
				ContainerToComponent: map[string]string{
					"postgres": "", // TODO verify container name
				},
			},
		},
		{
			Group:        "opentelemetry.io",
			Kind:         "OpenTelemetryCollector",
			ResourcePath: []string{"spec", "resources"},
			Match: MatchRule{
				WorkloadKind: "Deployment", // TODO verify: also StatefulSet/DaemonSet per spec.mode
				NameSuffix:   "-collector",
				ContainerToComponent: map[string]string{
					"otc-container": "", // TODO verify container name in deployment mode
				},
			},
		},
	}}
}
