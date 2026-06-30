// Package e2e exercises the assembled rere pipeline end-to-end through the CLI,
// using no mocks: KRR JSON -> adapter -> discover -> policy -> yamledit, with
// the result printed as a dry-run diff. No cluster and no credentials.
package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ca-moes/rere/internal/cli"
)

func TestDryRunDownsize(t *testing.T) {
	repo := filepath.Join("testdata", "gitops-repo")
	manifest := filepath.Join(repo, "deployment.yaml")

	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	root := cli.NewRootCommand("test", "none", "now")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{
		"run", "--dry-run",
		"--repo", repo,
		"--input", filepath.Join(repo, "krr.json"),
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("rere run --dry-run failed: %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	// The diff is real (produced by yamledit) and shows the downsize.
	if !strings.Contains(got, "@@") {
		t.Errorf("expected a unified diff, got:\n%s", got)
	}
	if !strings.Contains(got, "250m") {
		t.Errorf("diff missing downsized cpu (250m):\n%s", got)
	}
	if !strings.Contains(got, "deployment.yaml") {
		t.Errorf("diff missing file header:\n%s", got)
	}
	// The CPU limit should be removed (a deletion line carrying the old "2").
	if !strings.Contains(got, `-              cpu: "2"`) {
		t.Errorf("expected cpu limit removal in diff:\n%s", got)
	}

	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("dry-run modified the on-disk fixture")
	}
}
