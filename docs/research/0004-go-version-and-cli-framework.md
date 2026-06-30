---
id: RSCH-0004
type: research
title: Go version and CLI framework choice
status: completed
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev]
tags: [overview]
related:
  implements: []
  informed_by: []
  supersedes: []
  superseded_by: []
  see_also: [ADR-0005]
---

# RSCH-0004 — Go version and CLI framework choice

## Question

What Go version should `rere` target, and does it need a CLI framework (and which)?

## Findings

### Go version

- Latest stable is **Go 1.26.4** (2026-06-02); 1.26.0 shipped 2026-02-10; 1.25.x is the prior supported line. Go supports the **two newest majors** ([go.dev/doc/devel/release](https://go.dev/doc/devel/release)).
- The binding constraint is the k8s ecosystem `go` directive floor:
  - `k8s.io/apimachinery` → **1.26.0** ([go.mod](https://github.com/kubernetes/apimachinery/blob/master/go.mod))
  - `sigs.k8s.io/controller-runtime` (→v0.24) → **1.26.0** ([go.mod](https://github.com/kubernetes-sigs/controller-runtime/blob/main/go.mod))
  - `fluxcd/source-controller/api` → **1.26.0** ([go.mod](https://github.com/fluxcd/source-controller/blob/main/api/go.mod))
  - `sigs.k8s.io/kustomize/kyaml` → **1.25.0** ([go.mod](https://github.com/kubernetes-sigs/kustomize/blob/master/kyaml/go.mod))

The highest floor wins. **Target `go 1.26.0`** in `go.mod` (optionally a `toolchain go1.26.4` line) — it matches what current apimachinery/controller-runtime/flux already impose and stays in the supported window.

### CLI framework

Three categories get conflated here; only the first is the actual decision. **Arg/command parsers**: cobra, urfave/cli, kong, zeebo/clingy, stdlib `flag`. **Config loaders**: viper (file/env/flag layering) — complements a parser, isn't one. **TUI frameworks**: bubbletea (interactive terminal UIs) — orthogonal to arg parsing. All seven were weighed.

Parsers compared:

| Criterion | cobra v1.10.2 | urfave/cli v3.8.0 | kong v1.15.0 | zeebo/clingy | stdlib `flag` |
|---|---|---|---|---|---|
| Subcommands | First-class, nested | First-class | First-class (struct tags) | Yes (groups) | Manual dispatch |
| Persistent flags | Yes | Yes | Yes (embedding) | Yes (context) | Manual |
| Help/usage | Excellent | Good | Good (from tags) | Auto, basic | Bare |
| Shell completion | Built-in (bash/zsh/fish/ps) | Generated | Via `kongplete` | None | None |
| Config-file + flag layering | Via viper | Limited | Resolvers (yaml/env) | None | Manual |
| Dependency weight | Heaviest (+pflag) | Light–medium | Light | Zero deps | Zero |
| k8s-contributor familiarity | **Very high** | Medium | Low | None | High |
| Maturity | Active, ubiquitous | Active (Mar 2026) | Active (Apr 2026) | Niche (~12★, early) | Stdlib |

Sources: [cobra](https://github.com/spf13/cobra/releases) · [urfave/cli](https://github.com/urfave/cli/releases) · [kong](https://pkg.go.dev/github.com/alecthomas/kong?tab=versions) · [clingy](https://github.com/zeebo/clingy).

Not parsers (positioned, not rejected as parsers):

- **viper** ([repo](https://github.com/spf13/viper)) — a config solution (YAML/env/flag precedence) that pairs with cobra. Useful later for layered config; for M1 plain YAML unmarshal + explicit flag overrides is simpler and more testable, so viper is **deferred**.
- **bubbletea** ([repo](https://github.com/charmbracelet/bubbletea)) — an Elm-architecture **TUI** framework for interactive terminal apps. rere's core is non-interactive (CI/Action), so it is not the CLI framework; it could power an **optional future** interactive "review the proposed edits" mode, additively alongside cobra.

**Is a framework even needed?** For ~3 subcommands stdlib `flag` *can* work, but rere's needs push past its sweet spot: persistent flags across `run`/`diff`/`version` (+future `trace`/`discover`), config ⊕ flag layering, and shell completion all become hand-rolled, error-prone code. clingy is too niche (~12★, early-stage, no completion ecosystem) to bet a foundation on.

## Verdict

- **Go:** `go 1.26.0` in `go.mod`, `toolchain go1.26.4`.
- **CLI parser:** **cobra v1.10.2.** Decisive factor is ecosystem alignment — kubectl, helm, flux, kustomize all use cobra, so rere's k8s-native users get identical flag/help/completion conventions and completion is built-in. kong is the strongest challenger (clean struct tags, built-in config resolvers, lighter deps) and would win if minimizing dependencies were top priority; urfave/cli v3 is fine but less idiomatic here; clingy is too immature; stdlib `flag` is rejected (layering + completion + persistent-flag cost exceeds cobra's dependency cost).
- **Config:** plain YAML for M1; **viper** deferred until layered config is needed.
- **Interactive UX:** **bubbletea** is an optional future add for an interactive review mode — not part of M1.

Backs [ADR-0005](../adrs/0005-cobra-cli-framework.md).
