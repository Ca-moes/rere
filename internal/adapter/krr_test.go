package adapter

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "krr", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// wantQ asserts a quantity pointer against an expected canonical string; an
// empty want means the pointer must be nil (value not recommended).
func wantQ(t *testing.T, got *resource.Quantity, want string) {
	t.Helper()
	switch {
	case want == "" && got != nil:
		t.Errorf("expected nil quantity, got %q", got.String())
	case want != "" && got == nil:
		t.Errorf("expected %q, got nil", want)
	case want != "" && got != nil && got.String() != want:
		t.Errorf("quantity = %q, want %q", got.String(), want)
	}
}

func TestParseKRR_Basic(t *testing.T) {
	targets, err := ParseKRR(openFixture(t, "basic.json"))
	if err != nil {
		t.Fatalf("ParseKRR: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}
	g := targets[0]
	if g.Namespace != "default" || g.Kind != "Deployment" || g.Name != "web" || g.Container != "web" {
		t.Errorf("identity = %+v", g)
	}
	wantQ(t, g.Recommended.Requests.CPU, "250m")
	wantQ(t, g.Recommended.Requests.Mem, "128Mi")
	wantQ(t, g.Recommended.Limits.CPU, "") // null -> nil
	wantQ(t, g.Recommended.Limits.Mem, "128Mi")
	if err := g.Validate(); err != nil {
		t.Errorf("returned target invalid: %v", err)
	}
}

func TestParseKRR_MultiContainer(t *testing.T) {
	targets, err := ParseKRR(openFixture(t, "multi_container.json"))
	if err != nil {
		t.Fatalf("ParseKRR: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(targets))
	}
	if targets[0].Container != "app" || targets[1].Container != "sidecar" {
		t.Errorf("containers = %q, %q", targets[0].Container, targets[1].Container)
	}
	wantQ(t, targets[0].Recommended.Requests.CPU, "750m")
	wantQ(t, targets[0].Recommended.Requests.Mem, "384Mi")
	wantQ(t, targets[1].Recommended.Requests.Mem, "32Mi")
}

func TestParseKRR_Sentinels(t *testing.T) {
	targets, err := ParseKRR(openFixture(t, "sentinels.json"))
	if err != nil {
		t.Fatalf("ParseKRR: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}
	g := targets[0]
	wantQ(t, g.Recommended.Requests.CPU, "") // "?" -> nil
	wantQ(t, g.Recommended.Requests.Mem, "") // "unset" -> nil
	wantQ(t, g.Recommended.Limits.CPU, "")   // bare null -> nil
	wantQ(t, g.Recommended.Limits.Mem, "128Mi")
}

func TestParseKRR_BareValues(t *testing.T) {
	targets, err := ParseKRR(openFixture(t, "bare_values.json"))
	if err != nil {
		t.Fatalf("ParseKRR: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}
	g := targets[0]
	wantQ(t, g.Recommended.Requests.CPU, "500m")
	wantQ(t, g.Recommended.Requests.Mem, "1Gi")
	wantQ(t, g.Recommended.Limits.Mem, "1Gi")
}

func TestParseKRR_SkippedScans(t *testing.T) {
	for _, name := range []string{"grouped_job.json", "all_unset.json", "empty.json"} {
		targets, err := ParseKRR(openFixture(t, name))
		if err != nil {
			t.Fatalf("ParseKRR(%s): %v", name, err)
		}
		if len(targets) != 0 {
			t.Errorf("%s: got %d targets, want 0", name, len(targets))
		}
	}
}

func TestParseKRR_Malformed(t *testing.T) {
	_, err := ParseKRR(openFixture(t, "malformed.json"))
	if err == nil {
		t.Fatal("expected error for malformed json, got nil")
	}
}

func TestRunKRR(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake krr uses a /bin/sh script")
	}
	data, err := os.ReadFile(filepath.Join("testdata", "krr", "basic.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fakekrr.sh")
	content := "#!/bin/sh\ncat <<'KRR_JSON'\n" + string(data) + "\nKRR_JSON\n"
	if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	targets, err := RunKRR(t.Context(), "/bin/sh", []string{script})
	if err != nil {
		t.Fatalf("RunKRR: %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "web" {
		t.Fatalf("RunKRR targets = %+v", targets)
	}
}
