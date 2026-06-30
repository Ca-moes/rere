---
id: ADR-0005
type: adr
title: Cobra for the CLI
status: accepted
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev]
tags: [overview]
related:
  implements: []
  informed_by: [RSCH-0004]
  supersedes: []
  superseded_by: []
  see_also: [ADR-0001]
---

# ADR-0005 — Cobra for the CLI

## Context

`rere` is a CLI with a few subcommands (`run`, `diff`/dry-run, `version`, later `trace`/`discover`), persistent flags (`--config`, `--dry-run`, `--repo`, `--kubeconfig`, `--context`, `-v`), config-file ⊕ flag layering, and ideally shell completion. We need to decide whether a framework is warranted and which one.

## Decision

We will use **`spf13/cobra`** (v1.10.2) for the CLI. We will defer `viper` until layered config actually demands it; for M1, plain YAML config unmarshalling plus explicit flag overrides is simpler and more testable.

## Alternatives considered

Parsers:

- **stdlib `flag`** — rejected: persistent flags across subcommands, config-file ⊕ flag layering, and shell completion all become hand-rolled, error-prone code that exceeds cobra's dependency cost.
- **`alecthomas/kong`** — strong challenger (clean struct tags, built-in config resolvers, lighter deps); would win if minimizing dependencies were the top priority.
- **`urfave/cli` v3** — fine, but less idiomatic for a k8s-native audience.
- **`zeebo/clingy`** — rejected: niche and early-stage (~12★), zero-dep but no shell-completion ecosystem and no k8s-community familiarity.

Not parsers (considered and positioned, see [RSCH-0004](../research/0004-go-version-and-cli-framework.md)):

- **`spf13/viper`** — a config loader (file/env/flag layering), not an arg parser; pairs with cobra. Deferred (M1 uses plain YAML + flag overrides).
- **`charmbracelet/bubbletea`** — a TUI framework for interactive terminal apps, orthogonal to arg parsing. Candidate for an optional future interactive review mode, not the CLI decision.

## Consequences

- Ecosystem alignment: kubectl, helm, flux, and kustomize all use cobra, so contributors and users get identical flag/help/completion conventions, and completion is built-in.
- Heaviest dependency of the options (pulls `pflag`), accepted for the familiarity and feature set.
- The run loop depends on interfaces (`Discoverer`, `Opener`), keeping the cobra layer thin and the pipeline unit-testable.

## Confidence

High. Cobra is the de-facto standard for k8s CLIs and actively maintained. We would reconsider only if the dependency footprint became a real constraint (then kong). Informed by [RSCH-0004](../research/0004-go-version-and-cli-framework.md).
