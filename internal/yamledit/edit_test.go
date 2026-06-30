package yamledit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

// TestApplyGolden drives byte-exact golden files. Regenerate with
// UPDATE_GOLDEN=1 go test ./internal/yamledit/ and review the diff by hand.
func TestApplyGolden(t *testing.T) {
	cases := []struct {
		name        string
		kind        string
		mName       string
		edits       []Edit
		wantChanged bool
	}{
		{
			name: "comments", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []Edit{
				{Container: "web", Section: "requests", Resource: "cpu", Value: "250m"},
				{Container: "web", Section: "requests", Resource: "memory", Value: "128Mi"},
			},
		},
		{
			name: "anchors", kind: "Deployment", mName: "api", wantChanged: true,
			edits: []Edit{{Container: "api", Section: "requests", Resource: "cpu", Value: "200m"}},
		},
		{
			name: "multidoc", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []Edit{{Container: "web", Section: "requests", Resource: "cpu", Value: "250m"}},
		},
		{
			name: "multicontainer", kind: "Deployment", mName: "api", wantChanged: true,
			edits: []Edit{{Container: "app", Section: "requests", Resource: "cpu", Value: "750m"}},
		},
		{
			name: "cronjob", kind: "CronJob", mName: "report", wantChanged: true,
			edits: []Edit{{Container: "report", Section: "requests", Resource: "cpu", Value: "300m"}},
		},
		{
			name: "create", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []Edit{
				{Container: "web", Section: "requests", Resource: "cpu", Value: "250m"},
				{Container: "web", Section: "requests", Resource: "memory", Value: "128Mi"},
			},
		},
		{
			name: "removelimit", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []Edit{{Container: "web", Section: "limits", Resource: "cpu", Delete: true}},
		},
		{
			name: "removelimit_only", kind: "Deployment", mName: "web", wantChanged: true,
			edits: []Edit{{Container: "web", Section: "limits", Resource: "cpu", Delete: true}},
		},
		{
			name: "notfound", kind: "Deployment", mName: "web", wantChanged: false,
			edits: []Edit{{Container: "ghost", Section: "requests", Resource: "cpu", Value: "250m"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := readFixture(t, c.name+".in.yaml")
			var out bytes.Buffer
			changed, err := Apply(bytes.NewReader(in), &out, c.kind, c.mName, c.edits)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if changed != c.wantChanged {
				t.Errorf("changed = %v, want %v", changed, c.wantChanged)
			}
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

// TestApplyNotFoundBytesIdentical guarantees the byte-for-byte passthrough when
// nothing changes (container absent).
func TestApplyNotFoundBytesIdentical(t *testing.T) {
	in := readFixture(t, "notfound.in.yaml")
	var out bytes.Buffer
	changed, err := Apply(bytes.NewReader(in), &out, "Deployment", "web",
		[]Edit{{Container: "ghost", Section: "requests", Resource: "cpu", Value: "250m"}})
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

// TestApplyIdempotent: writing the value already present is a no-op.
func TestApplyIdempotent(t *testing.T) {
	in := readFixture(t, "notfound.in.yaml") // web/requests.cpu already 100m
	var out bytes.Buffer
	changed, err := Apply(bytes.NewReader(in), &out, "Deployment", "web",
		[]Edit{{Container: "web", Section: "requests", Resource: "cpu", Value: "100m"}})
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
