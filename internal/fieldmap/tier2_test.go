package fieldmap

import (
	"slices"
	"testing"
)

const cnpgYAML = `apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: mycluster
spec:
  instances: 3
  resources:
    requests:
      cpu: "1"
      memory: 1Gi
`

const otelYAML = `apiVersion: opentelemetry.io/v1beta1
kind: OpenTelemetryCollector
metadata:
  name: otel
spec:
  mode: deployment
  resources:
    requests:
      cpu: 100m
`

const myappYAML = `apiVersion: example.com/v1
kind: MyApp
metadata:
  name: myapp
spec:
  server:
    resources:
      requests:
        cpu: 100m
  worker:
    resources:
      requests:
        cpu: 50m
`

func myappMaps() MapConfig {
	return MapConfig{Maps: []CRMap{{
		Group: "example.com", Kind: "MyApp",
		Components: []Component{
			{Name: "server", Path: []string{"spec", "server", "resources"}},
			{Name: "worker", Path: []string{"spec", "worker", "resources"}},
		},
	}}}
}

func TestTier2_Supports(t *testing.T) {
	m := Tier2{Maps: BuiltinMaps()}
	if !m.Supports(parse(t, cnpgYAML)) {
		t.Error("CNPG Cluster should be supported")
	}
	if !m.Supports(parse(t, otelYAML)) {
		t.Error("OpenTelemetryCollector should be supported")
	}
	if m.Supports(parse(t, workloadYAML("Deployment"))) {
		t.Error("Deployment should not be supported by tier-2")
	}
	if m.Supports(parse(t, myappYAML)) {
		t.Error("unmapped CR should not be supported")
	}
}

func TestTier2_ResolveSingleComponent(t *testing.T) {
	m := Tier2{Maps: BuiltinMaps()}
	want := map[ResourceField]string{
		{Section: "requests", Name: "cpu"}:    "250m",
		{Section: "requests", Name: "memory"}: "512Mi",
		{Section: "limits", Name: "cpu"}:      "500m",
		{Section: "limits", Name: "memory"}:   "512Mi",
	}
	for _, tc := range []struct {
		name, yaml, kind string
	}{
		{"cnpg", cnpgYAML, "Cluster"},
		{"otel", otelYAML, "OpenTelemetryCollector"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edits, err := m.Resolve(parse(t, tc.yaml), Target{Kind: tc.kind}, want)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if len(edits) != 4 {
				t.Fatalf("got %d edits, want 4", len(edits))
			}
			for _, e := range edits {
				wantPath := []string{"spec", "resources", e.Field.Section, e.Field.Name}
				if !slices.Equal(e.Path, wantPath) {
					t.Errorf("path = %v, want %v", e.Path, wantPath)
				}
			}
			// deterministic (section, name) order
			if edits[0].Field != (ResourceField{Section: "limits", Name: "cpu"}) {
				t.Errorf("first edit = %v, want limits/cpu", edits[0].Field)
			}
		})
	}
}

func TestTier2_ResolveMultiComponent(t *testing.T) {
	m := Tier2{Maps: myappMaps()}
	root := parse(t, myappYAML)
	want := map[ResourceField]string{{Section: "requests", Name: "cpu"}: "200m"}

	server, err := m.Resolve(root, Target{Kind: "MyApp", Container: "server"}, want)
	if err != nil {
		t.Fatalf("server Resolve: %v", err)
	}
	if !slices.Equal(server[0].Path, []string{"spec", "server", "resources", "requests", "cpu"}) {
		t.Errorf("server path = %v", server[0].Path)
	}

	worker, err := m.Resolve(root, Target{Kind: "MyApp", Container: "worker"}, want)
	if err != nil {
		t.Fatalf("worker Resolve: %v", err)
	}
	if !slices.Equal(worker[0].Path, []string{"spec", "worker", "resources", "requests", "cpu"}) {
		t.Errorf("worker path = %v", worker[0].Path)
	}

	if _, err := m.Resolve(root, Target{Kind: "MyApp", Container: "ghost"}, want); err == nil {
		t.Error("unknown component should error")
	}
}

func TestTier2_ResolveMissingSubtree(t *testing.T) {
	// CNPG manifest with no spec at all -> the resources parent is absent.
	noSpec := "apiVersion: postgresql.cnpg.io/v1\nkind: Cluster\nmetadata:\n  name: x\n"
	m := Tier2{Maps: BuiltinMaps()}
	if _, err := m.ResolvePath(parse(t, noSpec), Target{Kind: "Cluster"},
		ResourceField{Section: "requests", Name: "cpu"}); err == nil {
		t.Error("expected error when the resources parent is absent")
	}

	// MyApp missing the worker component subtree.
	missingWorker := "apiVersion: example.com/v1\nkind: MyApp\nmetadata:\n  name: m\nspec:\n  server:\n    resources: {}\n"
	mm := Tier2{Maps: myappMaps()}
	if _, err := mm.ResolvePath(parse(t, missingWorker), Target{Kind: "MyApp", Container: "worker"},
		ResourceField{Section: "requests", Name: "cpu"}); err == nil {
		t.Error("expected error when the component subtree is absent")
	}
}
