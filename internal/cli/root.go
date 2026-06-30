// Package cli wires the cobra command tree and the pipeline run loop. The cobra
// layer is intentionally thin: it loads config, applies flag overrides, and
// hands off to Runner, which depends only on interfaces.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/Ca-moes/rere/internal/config"
	"github.com/Ca-moes/rere/internal/discover"
	"github.com/Ca-moes/rere/internal/pr"
)

type options struct {
	configPath string
	repoPath   string
	input      string
	dryRun     bool
	verbose    bool
}

// NewRootCommand builds the rere command tree.
func NewRootCommand(version, commit, date string) *cobra.Command {
	opts := &options{}
	root := &cobra.Command{
		Use:           "rere",
		Short:         "Write Kubernetes right-sizing recommendations back to a GitOps repo",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pf := root.PersistentFlags()
	pf.StringVar(&opts.configPath, "config", "", "config file (YAML)")
	pf.StringVar(&opts.repoPath, "repo", ".", "local checkout to read and edit")
	pf.BoolVar(&opts.dryRun, "dry-run", false, "print the diff and exit without writing")
	pf.BoolVarP(&opts.verbose, "verbose", "v", false, "verbose logging")

	root.AddCommand(newRunCommand(opts))
	root.AddCommand(newVersionCommand(version, commit, date))
	return root
}

func newRunCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Apply recommendations as PRs (or print diffs with --dry-run)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPipeline(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.input, "input", "i", "", "KRR -f json input (file or '-' for stdin); overrides config")
	return cmd
}

func runPipeline(ctx context.Context, out io.Writer, opts *options) error {
	slog.SetDefault(loggerFor(opts.verbose, os.Stderr))

	cfg, err := config.Parse(opts.configPath)
	if err != nil {
		return err
	}
	if opts.dryRun {
		cfg.DryRun = true
	}
	if opts.input != "" {
		cfg.Recommender.Input = opts.input
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	targets, err := readRecommendations(cfg)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, "no recommendations to apply")
		return nil
	}

	runner := &Runner{
		Cfg:        cfg,
		Repo:       opts.repoPath,
		Discoverer: &discover.RepoScanner{Root: opts.repoPath, Include: cfg.Discover.Include, Exclude: cfg.Discover.Exclude},
		Out:        out,
	}
	if !cfg.DryRun {
		token := os.Getenv(cfg.Git.Auth.TokenEnv)
		if token == "" {
			return fmt.Errorf("env %s is empty; cannot authenticate to GitHub", cfg.Git.Auth.TokenEnv)
		}
		runner.Opener = pr.NewGitHubOpenerFromToken(ctx, token)
	}
	return runner.Run(ctx, targets)
}
