// Package config loads and validates rere's YAML configuration, layered over
// built-in defaults, with flag overrides applied by the CLI.
package config

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/Ca-moes/rere/internal/policy"
)

// Config is the full rere configuration.
type Config struct {
	Recommender Recommender   `json:"recommender"`
	Git         Git           `json:"git"`
	Policy      policy.Config `json:"policy"`
	Discover    Discover      `json:"discover"`
	DryRun      bool          `json:"dryRun"`
}

// Recommender selects the input source and its location.
type Recommender struct {
	Source string `json:"source"` // "krr"
	Input  string `json:"input"`  // file path, or "-" for stdin
}

// Git configures the destination repository and PR behavior.
type Git struct {
	Repo         string `json:"repo"`         // owner/name
	BaseBranch   string `json:"baseBranch"`   // PR base
	BranchPrefix string `json:"branchPrefix"` // head branch prefix
	MergeMethod  string `json:"mergeMethod"`  // squash | merge | rebase
	AutoMerge    bool   `json:"autoMerge"`
	Auth         Auth   `json:"auth"`
}

// Auth resolves the GitHub token from an environment variable — never inline.
type Auth struct {
	Kind     string `json:"kind"`     // "pat"
	TokenEnv string `json:"tokenEnv"` // name of the env var holding the PAT
}

// Discover scopes the repo scan.
type Discover struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// Defaults returns the built-in configuration.
func Defaults() *Config {
	return &Config{
		Recommender: Recommender{Source: "krr", Input: "-"},
		Git: Git{
			BaseBranch:   "main",
			BranchPrefix: "rere/",
			MergeMethod:  "squash",
			AutoMerge:    true,
			Auth:         Auth{Kind: "pat", TokenEnv: "GITHUB_TOKEN"},
		},
		Policy: policy.Defaults(),
	}
}

// Parse reads a YAML config file over the defaults without validating. An empty
// path returns the defaults. The CLI uses this so it can apply flag overrides
// (e.g. --dry-run) before validating.
func Parse(path string) (*Config, error) {
	cfg := Defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	return cfg, nil
}

// Load parses a config file over the defaults and validates the result.
func Load(path string) (*Config, error) {
	cfg, err := Parse(path)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

var tokenPrefixes = []string{"ghp_", "gho_", "ghs_", "ghu_", "github_pat_"}

// Validate checks the configuration is internally consistent.
func (c *Config) Validate() error {
	if c.Recommender.Source != "krr" {
		return fmt.Errorf("recommender.source: only %q is supported in M1, got %q", "krr", c.Recommender.Source)
	}
	for _, p := range tokenPrefixes {
		if strings.HasPrefix(c.Git.Auth.TokenEnv, p) {
			return fmt.Errorf("git.auth.tokenEnv must be the NAME of an env var, not a token value")
		}
	}
	if !c.DryRun {
		if c.Git.Repo == "" {
			return fmt.Errorf("git.repo (owner/name) is required unless --dry-run")
		}
		if c.Git.Auth.TokenEnv == "" {
			return fmt.Errorf("git.auth.tokenEnv is required unless --dry-run")
		}
	}
	return nil
}
