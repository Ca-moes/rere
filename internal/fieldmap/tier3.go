package fieldmap

import (
	"fmt"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// helmReleaseGroup is the Flux HelmRelease apiVersion group (version-agnostic,
// like the tier-2 group match).
const helmReleaseGroup = "helm.toolkit.fluxcd.io"

// Tier3 handles Flux HelmReleases: it edits container resources inside the
// release's inline spec.values, at a chart-specific path. Charts is the merged
// registry (built-ins overlaid by user config); construct it with
// MergedChartMaps so the built-in chart maps are always present.
type Tier3 struct {
	Charts ChartConfig
}

// chartOf returns the chart a HelmRelease deploys (spec.chart.spec.chart), or ""
// when absent — e.g. a spec.chartRef release, which tier-3 does not map because
// the chart name it keys on is not inline.
func chartOf(root *yaml.RNode) string {
	n, err := root.Pipe(yaml.Lookup("spec", "chart", "spec", "chart"))
	if err != nil || n == nil {
		return ""
	}
	return yaml.GetValue(n)
}

// Supports reports whether root is a Flux HelmRelease whose chart has a map.
func (m Tier3) Supports(root *yaml.RNode) bool {
	group, kind := groupKind(root)
	if group != helmReleaseGroup || kind != "HelmRelease" {
		return false
	}
	return findChartMap(m.Charts, chartOf(root)) != nil
}

// ResolvePath resolves one resource cell within the release's spec.values, after
// verifying the resources block's parent exists (the resources map itself may be
// created, but never its parent — no phantom subtrees under values).
func (m Tier3) ResolvePath(root *yaml.RNode, t Target, f ResourceField) ([]string, error) {
	chart := chartOf(root)
	cm := findChartMap(m.Charts, chart)
	if cm == nil {
		return nil, fmt.Errorf("fieldmap: no tier-3 map for chart %q", chart)
	}
	base, err := valuesBase(cm, t)
	if err != nil {
		return nil, err
	}
	if parent := base[:len(base)-1]; len(parent) > 0 {
		n, err := root.Pipe(yaml.Lookup(parent...))
		if err != nil {
			return nil, fmt.Errorf("fieldmap: lookup %v in chart %q: %w", parent, chart, err)
		}
		if n == nil {
			return nil, fmt.Errorf("fieldmap: chart %q: values parent %v not found", chart, parent)
		}
	}
	return append(append([]string{}, base...), f.Section, f.Name), nil
}

// Resolve returns a ResolvedEdit per wanted field in deterministic order.
func (m Tier3) Resolve(root *yaml.RNode, t Target, want map[ResourceField]string) ([]ResolvedEdit, error) {
	return resolveWant(func(f ResourceField) ([]string, error) {
		return m.ResolvePath(root, t, f)
	}, want)
}

// valuesBase is the path to a chart's resources mapping under spec.values. For
// multi-component charts the Target.Container is the component name
// (TranslateHelmTarget already mapped the reported workload to it).
func valuesBase(cm *ChartMap, t Target) ([]string, error) {
	if len(cm.Components) == 0 {
		return cm.ResourcePath, nil
	}
	for i := range cm.Components {
		if cm.Components[i].Name == t.Container {
			return cm.Components[i].Path, nil
		}
	}
	return nil, fmt.Errorf("fieldmap: chart %q: no component named %q", cm.Chart, t.Container)
}
