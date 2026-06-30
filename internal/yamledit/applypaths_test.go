package yamledit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// tier1Path builds the absolute resolved path a tier-1 FieldMapper produces, so
// the path-addressed editor can be proven byte-identical to the legacy Edit path
// against the very same golden fixtures.
func tier1Path(kind, container, section, resource string) []string {
	base := []string{"spec", "template", "spec"}
	if kind == "CronJob" {
		base = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	return append(base, "containers", "[name="+container+"]", "resources", section, resource)
}

// TestApplyPathsGolden drives the path-addressed editor through the existing
// tier-1 golden files. It MUST pass without UPDATE_GOLDEN — any byte difference
// means the generalization diverged from the legacy editor's kyaml op order.
func TestApplyPathsGolden(t *testing.T) {
	cases := []struct {
		name        string
		kind        string
		mName       string
		edits       []PathEdit
		wantChanged bool
	}{
		{
			name: "comments", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []PathEdit{
				{Path: tier1Path("Deployment", "web", "requests", "cpu"), Value: "250m"},
				{Path: tier1Path("Deployment", "web", "requests", "memory"), Value: "128Mi"},
			},
		},
		{
			name: "anchors", kind: "Deployment", mName: "api", wantChanged: true,
			edits: []PathEdit{{Path: tier1Path("Deployment", "api", "requests", "cpu"), Value: "200m"}},
		},
		{
			name: "multidoc", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []PathEdit{{Path: tier1Path("Deployment", "web", "requests", "cpu"), Value: "250m"}},
		},
		{
			name: "multicontainer", kind: "Deployment", mName: "api", wantChanged: true,
			edits: []PathEdit{{Path: tier1Path("Deployment", "app", "requests", "cpu"), Value: "750m"}},
		},
		{
			name: "cronjob", kind: "CronJob", mName: "report", wantChanged: true,
			edits: []PathEdit{{Path: tier1Path("CronJob", "report", "requests", "cpu"), Value: "300m"}},
		},
		{
			name: "create", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []PathEdit{
				{Path: tier1Path("Deployment", "web", "requests", "cpu"), Value: "250m"},
				{Path: tier1Path("Deployment", "web", "requests", "memory"), Value: "128Mi"},
			},
		},
		{
			name: "removelimit", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []PathEdit{{Path: tier1Path("Deployment", "web", "limits", "cpu"), Delete: true}},
		},
		{
			name: "removelimit_only", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []PathEdit{{Path: tier1Path("Deployment", "web", "limits", "cpu"), Delete: true}},
		},
		{
			name: "notfound", kind: "Deployment", mName: "web", wantChanged: false,
			edits: []PathEdit{{Path: tier1Path("Deployment", "ghost", "requests", "cpu"), Value: "250m"}},
		},
		// Tier-2 operator CRs: resources at a config-driven path (no container
		// selector). These prove comments and large sibling blocks survive.
		{
			name: "cnpg", kind: "Cluster", mName: "mycluster", wantChanged: true,
			edits: []PathEdit{
				{Path: []string{"spec", "resources", "requests", "cpu"}, Value: "250m"},
				{Path: []string{"spec", "resources", "requests", "memory"}, Value: "295Mi"},
				{Path: []string{"spec", "resources", "limits", "cpu"}, Delete: true},
				{Path: []string{"spec", "resources", "limits", "memory"}, Value: "295Mi"},
			},
		},
		{
			name: "otelcol", kind: "OpenTelemetryCollector", mName: "otel", wantChanged: true,
			edits: []PathEdit{
				{Path: []string{"spec", "resources", "requests", "cpu"}, Value: "250m"},
				{Path: []string{"spec", "resources", "requests", "memory"}, Value: "295Mi"},
			},
		},
		{
			name: "crcreate", kind: "Cluster", mName: "fresh", wantChanged: true,
			edits: []PathEdit{
				{Path: []string{"spec", "resources", "requests", "cpu"}, Value: "250m"},
				{Path: []string{"spec", "resources", "requests", "memory"}, Value: "295Mi"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := readFixture(t, c.name+".in.yaml")
			var out bytes.Buffer
			changed, err := ApplyPaths(bytes.NewReader(in), &out, c.kind, c.mName, c.edits)
			if err != nil {
				t.Fatalf("ApplyPaths: %v", err)
			}
			if changed != c.wantChanged {
				t.Errorf("changed = %v, want %v", changed, c.wantChanged)
			}
			// Regenerate with UPDATE_GOLDEN=1 go test ./internal/yamledit/ and
			// review the diff by hand. Tier-1 goldens must never change.
			goldenPath := filepath.Join("testdata", c.name+".out.yaml")
			if os.Getenv("UPDATE_GOLDEN") != "" {
				if err := os.WriteFile(goldenPath, out.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want := readFixture(t, c.name+".out.yaml")
			if !bytes.Equal(out.Bytes(), want) {
				t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.Bytes(), want)
			}
		})
	}
}

// TestApplyPathsNotFoundBytesIdentical: a path whose sequence-element ancestor
// (the container) is absent is a no-op with byte-for-byte passthrough — the
// editor must never fabricate a missing list entry.
func TestApplyPathsNotFoundBytesIdentical(t *testing.T) {
	in := readFixture(t, "notfound.in.yaml")
	var out bytes.Buffer
	changed, err := ApplyPaths(bytes.NewReader(in), &out, "Deployment", "web",
		[]PathEdit{{Path: tier1Path("Deployment", "ghost", "requests", "cpu"), Value: "250m"}})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("changed = true, want false")
	}
	if !bytes.Equal(out.Bytes(), in) {
		t.Errorf("passthrough not byte-identical:\n got: %q\nwant: %q", out.Bytes(), in)
	}
}

// TestApplyPathsIdempotent: writing the value already present is a no-op.
func TestApplyPathsIdempotent(t *testing.T) {
	in := readFixture(t, "notfound.in.yaml") // web/requests.cpu already 100m
	var out bytes.Buffer
	changed, err := ApplyPaths(bytes.NewReader(in), &out, "Deployment", "web",
		[]PathEdit{{Path: tier1Path("Deployment", "web", "requests", "cpu"), Value: "100m"}})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("re-applying the same value reported changed = true")
	}
	if !bytes.Equal(out.Bytes(), in) {
		t.Error("idempotent apply changed bytes")
	}
}
