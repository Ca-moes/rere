package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Defaults()
	if c.Recommender.Source != "krr" || c.Recommender.Input != "-" {
		t.Errorf("recommender defaults = %+v", c.Recommender)
	}
	if c.Git.BaseBranch != "main" || c.Git.BranchPrefix != "rere/" || c.Git.MergeMethod != "squash" || !c.Git.AutoMerge {
		t.Errorf("git defaults = %+v", c.Git)
	}
	if c.Git.Auth.TokenEnv != "GITHUB_TOKEN" {
		t.Errorf("auth defaults = %+v", c.Git.Auth)
	}
	if c.Policy.DeadbandPct != 0.10 || c.Policy.MemHeadroom != 1.15 {
		t.Errorf("policy defaults = %+v", c.Policy)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rere.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidate_FieldMaps(t *testing.T) {
	// Neither resourcePath nor components -> invalid.
	bad := writeConfig(t, `
recommender:
  source: krr
dryRun: true
fieldMaps:
  maps:
    - group: example.com
      kind: Foo
`)
	if _, err := Load(bad); err == nil {
		t.Error("expected invalid fieldMaps to fail Load")
	}

	// A well-formed user map parses (and merges over built-ins at runtime).
	good := writeConfig(t, `
recommender:
  source: krr
dryRun: true
fieldMaps:
  maps:
    - group: example.com
      kind: Foo
      resourcePath: [spec, resources]
`)
	cfg, err := Load(good)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.FieldMaps.Maps) != 1 || cfg.FieldMaps.Maps[0].Kind != "Foo" {
		t.Errorf("fieldMaps not parsed: %+v", cfg.FieldMaps)
	}
}

func TestLoad_OverridesAndKeepsDefaults(t *testing.T) {
	p := writeConfig(t, `
git:
  repo: acme/widgets
  baseBranch: develop
policy:
  deadbandPct: 0.2
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Git.Repo != "acme/widgets" || c.Git.BaseBranch != "develop" {
		t.Errorf("overrides not applied: %+v", c.Git)
	}
	// Untouched fields keep defaults.
	if c.Git.MergeMethod != "squash" || !c.Git.AutoMerge {
		t.Errorf("defaults lost: %+v", c.Git)
	}
	if c.Policy.DeadbandPct != 0.2 {
		t.Errorf("policy override lost: %v", c.Policy.DeadbandPct)
	}
	if c.Policy.MemHeadroom != 1.15 {
		t.Errorf("policy default lost: %v", c.Policy.MemHeadroom)
	}
}

func TestValidate_RejectsInlineToken(t *testing.T) {
	p := writeConfig(t, `
git:
  repo: acme/widgets
  auth:
    tokenEnv: ghp_realtokenlookingvalue000000000000000000
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for inline-looking token in tokenEnv")
	}
}

func TestValidate_LiveRequiresRepo(t *testing.T) {
	p := writeConfig(t, `
git:
  repo: ""
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error: live mode requires git.repo")
	}
}

func TestValidate_DryRunRelaxesRequirements(t *testing.T) {
	p := writeConfig(t, `
dryRun: true
git:
  repo: ""
`)
	if _, err := Load(p); err != nil {
		t.Fatalf("dry-run should not require repo: %v", err)
	}
}

func TestValidate_UnsupportedRecommender(t *testing.T) {
	p := writeConfig(t, `
dryRun: true
recommender:
  source: vpa
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unsupported recommender source")
	}
}

func TestValidate_MergeMethod(t *testing.T) {
	// A typo would otherwise fall through to the server's default merge method
	// silently (automerge picks no method for unknown strings).
	bad := writeConfig(t, `
dryRun: true
git:
  mergeMethod: sqaush
`)
	if _, err := Load(bad); err == nil {
		t.Error("expected error for unknown git.mergeMethod")
	}
	for _, m := range []string{"merge", "squash", "rebase"} {
		p := writeConfig(t, "dryRun: true\ngit:\n  mergeMethod: "+m+"\n")
		if _, err := Load(p); err != nil {
			t.Errorf("mergeMethod %q should validate: %v", m, err)
		}
	}
}

func TestValidate_LiveRequiresOwnerNameRepo(t *testing.T) {
	// Shape is checked at Validate, not after all the edit work at PR time.
	p := writeConfig(t, `
git:
  repo: just-a-name
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error: git.repo must be owner/name")
	}
}

func TestValidate_LiveRequiresBaseBranch(t *testing.T) {
	p := writeConfig(t, `
git:
  repo: acme/widgets
  baseBranch: ""
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error: live mode requires git.baseBranch")
	}
}
