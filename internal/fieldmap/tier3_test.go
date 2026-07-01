package fieldmap

import (
	"slices"
	"testing"
)

const ingressHR = `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: ingress-nginx
spec:
  chart:
    spec:
      chart: ingress-nginx
      sourceRef:
        kind: HelmRepository
        name: ingress-nginx
  values:
    controller:
      resources:
        requests:
          cpu: 100m
    defaultBackend:
      resources:
        requests:
          cpu: 10m
`

const keycloakxHR = `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: keycloakx
spec:
  chart:
    spec:
      chart: keycloakx
  values:
    resources:
      requests:
        cpu: 500m
`

// chartRefHR uses spec.chartRef, so there is no inline chart name to match.
const chartRefHR = `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: podinfo
spec:
  chartRef:
    kind: OCIRepository
    name: podinfo
  values:
    resources: {}
`

// unmappedHR is a HelmRelease for a chart with no built-in or user map.
const unmappedHR = `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: redis
spec:
  chart:
    spec:
      chart: redis
  values:
    resources: {}
`

func TestTier3_Supports(t *testing.T) {
	m := Tier3{Charts: BuiltinChartMaps()}
	if !m.Supports(parse(t, ingressHR)) {
		t.Error("ingress-nginx HelmRelease should be supported")
	}
	if !m.Supports(parse(t, keycloakxHR)) {
		t.Error("keycloakx HelmRelease should be supported")
	}
	if m.Supports(parse(t, unmappedHR)) {
		t.Error("a HelmRelease for an unmapped chart should not be supported")
	}
	if m.Supports(parse(t, chartRefHR)) {
		t.Error("a chartRef HelmRelease (no inline chart name) should not be supported")
	}
	if m.Supports(parse(t, workloadYAML("Deployment"))) {
		t.Error("a Deployment should not be supported by tier-3")
	}
}

func TestTier3_ResolveSingleComponent(t *testing.T) {
	m := Tier3{Charts: BuiltinChartMaps()}
	want := map[ResourceField]string{
		{Section: "requests", Name: "cpu"}:    "250m",
		{Section: "requests", Name: "memory"}: "512Mi",
		{Section: "limits", Name: "cpu"}:      "500m",
		{Section: "limits", Name: "memory"}:   "512Mi",
	}
	edits, err := m.Resolve(parse(t, keycloakxHR), Target{Kind: "HelmRelease"}, want)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(edits) != 4 {
		t.Fatalf("got %d edits, want 4", len(edits))
	}
	for _, e := range edits {
		wantPath := []string{"spec", "values", "resources", e.Field.Section, e.Field.Name}
		if !slices.Equal(e.Path, wantPath) {
			t.Errorf("path = %v, want %v", e.Path, wantPath)
		}
	}
	// deterministic (section, name) order: limits/cpu first
	if edits[0].Field != (ResourceField{Section: "limits", Name: "cpu"}) {
		t.Errorf("first edit = %v, want limits/cpu", edits[0].Field)
	}
}

func TestTier3_ResolveMultiComponent(t *testing.T) {
	m := Tier3{Charts: BuiltinChartMaps()}
	root := parse(t, ingressHR)
	want := map[ResourceField]string{{Section: "requests", Name: "cpu"}: "200m"}

	ctrl, err := m.Resolve(root, Target{Kind: "HelmRelease", Container: "controller"}, want)
	if err != nil {
		t.Fatalf("controller Resolve: %v", err)
	}
	if !slices.Equal(ctrl[0].Path, []string{"spec", "values", "controller", "resources", "requests", "cpu"}) {
		t.Errorf("controller path = %v", ctrl[0].Path)
	}

	db, err := m.Resolve(root, Target{Kind: "HelmRelease", Container: "defaultBackend"}, want)
	if err != nil {
		t.Fatalf("defaultBackend Resolve: %v", err)
	}
	if !slices.Equal(db[0].Path, []string{"spec", "values", "defaultBackend", "resources", "requests", "cpu"}) {
		t.Errorf("defaultBackend path = %v", db[0].Path)
	}

	if _, err := m.Resolve(root, Target{Kind: "HelmRelease", Container: "ghost"}, want); err == nil {
		t.Error("unknown component should error")
	}
}

func TestTier3_ResolveMissingParent(t *testing.T) {
	m := Tier3{Charts: BuiltinChartMaps()}
	f := ResourceField{Section: "requests", Name: "cpu"}

	// keycloakx HelmRelease with no spec.values at all: the values parent is absent.
	noValues := "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: k\nspec:\n  chart:\n    spec:\n      chart: keycloakx\n"
	if _, err := m.ResolvePath(parse(t, noValues), Target{Kind: "HelmRelease"}, f); err == nil {
		t.Error("expected error when spec.values is absent")
	}

	// ingress-nginx with values but no controller subtree.
	noController := "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: i\nspec:\n  chart:\n    spec:\n      chart: ingress-nginx\n  values:\n    defaultBackend:\n      resources: {}\n"
	if _, err := m.ResolvePath(parse(t, noController), Target{Kind: "HelmRelease", Container: "controller"}, f); err == nil {
		t.Error("expected error when the controller subtree is absent")
	}
}

func TestTier3_ResolveCreatesResourcesUnderExistingParent(t *testing.T) {
	// spec.values.controller exists but has no resources block yet: the resources
	// map may be created, so ResolvePath succeeds (parent present).
	m := Tier3{Charts: BuiltinChartMaps()}
	hr := "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: i\nspec:\n  chart:\n    spec:\n      chart: ingress-nginx\n  values:\n    controller:\n      replicaCount: 2\n"
	got, err := m.ResolvePath(parse(t, hr), Target{Kind: "HelmRelease", Container: "controller"},
		ResourceField{Section: "requests", Name: "cpu"})
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want := []string{"spec", "values", "controller", "resources", "requests", "cpu"}
	if !slices.Equal(got, want) {
		t.Errorf("path = %v, want %v", got, want)
	}
}
