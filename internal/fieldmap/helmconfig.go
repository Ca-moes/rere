package fieldmap

import (
	"fmt"
	"regexp"
)

// ChartConfig is the tier-3 field-map registry: built-in per-chart maps overlaid
// by user-provided maps. It tells the tier-3 mapper where a Flux HelmRelease's
// chart keeps its container resources under spec.values, and how to translate a
// recommender's generated-workload identity back to the owning HelmRelease.
type ChartConfig struct {
	Maps []ChartMap `json:"maps"`
}

// ChartMap maps one Helm chart (identified by spec.chart.spec.chart) to where its
// resources live under spec.values. Single-workload charts use ResourcePath;
// charts that render several resource-bearing workloads use Components for
// name-aware selection. Exactly one of the two must be set.
type ChartMap struct {
	Chart        string           `json:"chart"`                  // chart name, e.g. "ingress-nginx"
	ResourcePath []string         `json:"resourcePath,omitempty"` // absolute from doc root, e.g. [spec, values, resources]
	Components   []ChartComponent `json:"components,omitempty"`
	Match        MatchRule        `json:"match,omitempty"` // single-workload translation rule

	// nameRE is the compiled Match.NamePattern, cached by MergedChartMaps so
	// translation does not recompile it per target. Unexported: not serialized.
	nameRE *regexp.Regexp
}

// ChartComponent is one named resource-bearing workload a multi-workload chart
// renders (e.g. ingress-nginx's controller and defaultBackend). Unlike a tier-2
// Component it carries its own Match, because each component is a separate
// generated workload with a distinct name: its rule recovers the shared release
// name and identifies this component.
type ChartComponent struct {
	Name  string    `json:"name"`
	Path  []string  `json:"path"` // absolute from doc root, e.g. [spec, values, controller, resources]
	Match MatchRule `json:"match,omitempty"`

	nameRE *regexp.Regexp
}

// Validate checks every map is well-formed and that chart is unique.
func (c ChartConfig) Validate() error {
	seen := map[string]bool{}
	for i, m := range c.Maps {
		if m.Chart == "" {
			return fmt.Errorf("helmReleaseMaps.maps[%d]: chart is required", i)
		}
		hasPath := len(m.ResourcePath) > 0
		hasComp := len(m.Components) > 0
		if hasPath == hasComp {
			return fmt.Errorf("helmReleaseMaps.maps[%d] (%s): set exactly one of resourcePath or components", i, m.Chart)
		}
		if err := validateNamePattern(m.Match); err != nil {
			return fmt.Errorf("helmReleaseMaps.maps[%d] (%s): %w", i, m.Chart, err)
		}
		for j, comp := range m.Components {
			if comp.Name == "" || len(comp.Path) == 0 {
				return fmt.Errorf("helmReleaseMaps.maps[%d].components[%d]: name and path are required", i, j)
			}
			if err := validateNamePattern(comp.Match); err != nil {
				return fmt.Errorf("helmReleaseMaps.maps[%d].components[%d] (%s): %w", i, j, comp.Name, err)
			}
		}
		if seen[m.Chart] {
			return fmt.Errorf("helmReleaseMaps: duplicate map for chart %q", m.Chart)
		}
		seen[m.Chart] = true
	}
	return nil
}

// findChartMap returns the map for a chart name, or nil.
func findChartMap(c ChartConfig, chart string) *ChartMap {
	for i := range c.Maps {
		if c.Maps[i].Chart == chart {
			return &c.Maps[i]
		}
	}
	return nil
}

// MergedChartMaps overlays user maps on the built-ins, mirroring MergedMaps: user
// maps come FIRST so they win in translation, a built-in is dropped when a user
// map shares its chart, and every NamePattern (top-level and per-component) is
// compiled once and cached.
func MergedChartMaps(user ChartConfig) ChartConfig {
	out := ChartConfig{Maps: append([]ChartMap{}, user.Maps...)}
	for _, bm := range BuiltinChartMaps().Maps {
		if findChartMap(out, bm.Chart) == nil {
			out.Maps = append(out.Maps, bm)
		}
	}
	for i := range out.Maps {
		out.Maps[i].nameRE = compileNameRE(out.Maps[i].Match)
		for j := range out.Maps[i].Components {
			out.Maps[i].Components[j].nameRE = compileNameRE(out.Maps[i].Components[j].Match)
		}
	}
	return out
}

// compileNameRE compiles a match rule's NamePattern, or returns nil when unset.
// A stray bad pattern leaves nil and recoverName falls back to a lazy compile;
// Validate rejects bad patterns before this runs.
func compileNameRE(m MatchRule) *regexp.Regexp {
	if m.NamePattern == "" {
		return nil
	}
	re, _ := regexp.Compile(m.NamePattern)
	return re
}

// BuiltinChartMaps are the Helm charts rere maps out of the box, so a config-less
// binary right-sizes the common ones.
//
// NOTE: the Match rules below are best-effort and not yet verified against a real
// recommender run — Helm's generated-workload names depend on the release name
// and fullname overrides (see #33). A wrong rule degrades safely: the workload
// falls back to tier-1, or the rewritten HelmRelease is not found in the repo and
// reverts to the untranslated workload.
func BuiltinChartMaps() ChartConfig {
	return ChartConfig{Maps: []ChartMap{
		{
			// keycloakx renders a single StatefulSet; resources at
			// spec.values.resources.
			Chart:        "keycloakx",
			ResourcePath: []string{"spec", "values", "resources"},
			Match: MatchRule{
				WorkloadKind: "StatefulSet", // TODO verify (#33)
				NameSuffix:   "-keycloakx",  // TODO verify (#33)
			},
		},
		{
			// ingress-nginx keeps per-component resources at
			// spec.values.<component>.resources; the controller and defaultBackend
			// are separate generated Deployments.
			Chart: "ingress-nginx",
			Components: []ChartComponent{
				{
					Name: "controller",
					Path: []string{"spec", "values", "controller", "resources"},
					Match: MatchRule{
						WorkloadKind: "Deployment", // TODO verify (#33): DaemonSet in host-network mode
						NameSuffix:   "-controller",
						// "-controller" is a generic suffix, so gate on the chart's
						// controller container name to avoid rewriting an unrelated
						// Deployment onto this HelmRelease.
						ContainerToComponent: map[string]string{"controller": ""}, // TODO verify (#33)
					},
				},
				{
					Name: "defaultBackend",
					Path: []string{"spec", "values", "defaultBackend", "resources"},
					Match: MatchRule{
						WorkloadKind: "Deployment",      // TODO verify (#33)
						NameSuffix:   "-defaultbackend", // TODO verify (#33)
					},
				},
			},
		},
	}}
}
