# CLAUDE.md — agent briefing for `rere`

`rere` (ré-ré = **re**source **re**sizer) is an MIT Go CLI + GitHub Action: the missing *write-back* half of Kubernetes resource right-sizing. Recommenders (KRR, VPA) compute correct CPU/memory requests & limits; `rere` safely writes those numbers back into a GitOps repo as clean, auto-merged PRs — across raw manifests, Helm `values:`, and operator CRs — via surgical, comment-preserving YAML edits (kyaml).

Read before non-trivial work:

- [docs/explanation/architecture.md](docs/explanation/architecture.md) — what rere is, the pipeline, the components, the tiers, scope.
- [docs/adrs/](docs/adrs/) — the settled decisions (Go+kyaml, don't-fork-Renovate, Flux-provenance discovery, local-checkout + GitHub-API write, CLI framework).
- [docs/research/](docs/research/) — the evidence behind those decisions.

## Pipeline

`KRR JSON → adapter → discover (owning Flux Kustomization/HelmRelease + repo path) → fieldmap (resolve field path) → policy (deadband/headroom/limits) → yamledit (kyaml surgical edit) → pr (branch/commit/PR + auto-merge)`. Code under `cmd/rere` + `internal/{adapter,discover,fieldmap,policy, yamledit,pr,cli,config}`.

## Conventions

- **Docs follow the Promptly docs-framework.** Every file under `docs/` must pass `docs-framework validate docs/` (run via `uvx`, see [docs/README.md](docs/README.md)). No CI docs gating — the framework's Action is in a private repo we don't run from here.
- **IDs are per-repo, monotonic per type** (`ADR-NNNN`, `RSCH-NNNN`). Research → ADR → explanation.
- **YAML edits are surgical.** Never whole-doc re-marshal; preserve comments/anchors/order (golden-file tested). This is the quality bar.
- **TDD.** Failing test first, then implement. The kyaml editor is golden-file tested.
- KRR `-f json` emits raw floats (CPU cores, memory bytes) — convert via `resource.Quantity`.
