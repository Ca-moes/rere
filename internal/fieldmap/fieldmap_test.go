package fieldmap

import (
	"slices"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func parse(t *testing.T, s string) *yaml.RNode {
	t.Helper()
	n, err := yaml.Parse(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return n
}

const deployYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      initContainers:
        - name: init
      containers:
        - name: web
        - name: sidecar
`

func workloadYAML(kind string) string {
	return "apiVersion: apps/v1\nkind: " + kind + `
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: web
`
}

func TestTier1_Supports(t *testing.T) {
	m := Tier1{}
	for _, k := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		if !m.Supports(parse(t, workloadYAML(k))) {
			t.Errorf("%s should be supported", k)
		}
	}
	for _, k := range []string{"CronJob", "Pod", "Job", "Rollout"} {
		if m.Supports(parse(t, workloadYAML(k))) {
			t.Errorf("%s should not be supported", k)
		}
	}
	// A non-resource doc (e.g. Helm values) has no kind.
	if m.Supports(parse(t, "replicaCount: 2\n")) {
		t.Error("values doc should not be supported")
	}
}

func TestTier1_ResolveAllFields(t *testing.T) {
	want := map[ResourceField]string{
		{Section: "requests", Name: "cpu"}:    "250m",
		{Section: "requests", Name: "memory"}: "128Mi",
		{Section: "limits", Name: "cpu"}:      "500m",
		{Section: "limits", Name: "memory"}:   "256Mi",
	}
	for _, kind := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		root := parse(t, workloadYAML(kind))
		edits, err := Tier1{}.Resolve(root, Target{Kind: kind, Container: "web"}, want)
		if err != nil {
			t.Fatalf("%s Resolve: %v", kind, err)
		}
		if len(edits) != 4 {
			t.Fatalf("%s: got %d edits, want 4", kind, len(edits))
		}
		for _, e := range edits {
			wantPath := []string{"spec", "template", "spec", "containers", "[name=web]", "resources", e.Field.Section, e.Field.Name}
			if !slices.Equal(e.Path, wantPath) {
				t.Errorf("%s path = %v, want %v", kind, e.Path, wantPath)
			}
			if e.Value != want[e.Field] {
				t.Errorf("%s value for %v = %q, want %q", kind, e.Field, e.Value, want[e.Field])
			}
		}
	}
}

func TestTier1_ResolveDeterministicOrder(t *testing.T) {
	want := map[ResourceField]string{
		{Section: "limits", Name: "memory"}:   "256Mi",
		{Section: "requests", Name: "cpu"}:    "250m",
		{Section: "limits", Name: "cpu"}:      "500m",
		{Section: "requests", Name: "memory"}: "128Mi",
	}
	edits, err := Tier1{}.Resolve(parse(t, deployYAML), Target{Kind: "Deployment", Container: "web"}, want)
	if err != nil {
		t.Fatal(err)
	}
	var got []ResourceField
	for _, e := range edits {
		got = append(got, e.Field)
	}
	wantOrder := []ResourceField{
		{Section: "limits", Name: "cpu"}, {Section: "limits", Name: "memory"},
		{Section: "requests", Name: "cpu"}, {Section: "requests", Name: "memory"},
	}
	if !slices.Equal(got, wantOrder) {
		t.Errorf("order = %v, want %v", got, wantOrder)
	}
}

func TestTier1_ResolvePath(t *testing.T) {
	m := Tier1{}
	root := parse(t, deployYAML)
	got, err := m.ResolvePath(root, Target{Kind: "Deployment", Container: "web"},
		ResourceField{Section: "requests", Name: "cpu"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"spec", "template", "spec", "containers", "[name=web]", "resources", "requests", "cpu"}
	if !slices.Equal(got, want) {
		t.Errorf("path = %v, want %v", got, want)
	}

	gotInit, err := m.ResolvePath(root,
		Target{Kind: "Deployment", Container: "init", InitContainer: true},
		ResourceField{Section: "limits", Name: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	wantInit := []string{"spec", "template", "spec", "initContainers", "[name=init]", "resources", "limits", "memory"}
	if !slices.Equal(gotInit, wantInit) {
		t.Errorf("init path = %v, want %v", gotInit, wantInit)
	}

	if _, err := m.ResolvePath(root, Target{Kind: "Deployment", Container: "ghost"},
		ResourceField{Section: "requests", Name: "cpu"}); err == nil {
		t.Error("expected error for absent container")
	}
}

func TestTier1_ResolveAbsentContainer(t *testing.T) {
	_, err := Tier1{}.Resolve(parse(t, deployYAML), Target{Kind: "Deployment", Container: "ghost"},
		map[ResourceField]string{{Section: "requests", Name: "cpu"}: "250m"})
	if err == nil {
		t.Fatal("expected error for absent container")
	}
}

func TestTier1_ResolveInitContainer(t *testing.T) {
	edits, err := Tier1{}.Resolve(parse(t, deployYAML),
		Target{Kind: "Deployment", Container: "init", InitContainer: true},
		map[ResourceField]string{{Section: "requests", Name: "cpu"}: "100m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1", len(edits))
	}
	wantPath := []string{"spec", "template", "spec", "initContainers", "[name=init]", "resources", "requests", "cpu"}
	if !slices.Equal(edits[0].Path, wantPath) {
		t.Errorf("path = %v, want %v", edits[0].Path, wantPath)
	}
}
