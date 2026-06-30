---
id: ADR-0002
type: adr
title: Build a standalone write-back tool; do not fork Renovate
status: accepted
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev, platform-engineer]
tags: [renovate, right-sizing]
related:
  implements: []
  informed_by: [RSCH-0002, RSCH-0001]
  supersedes: []
  superseded_by: []
  see_also: [ADR-0004]
---

# ADR-0002 — Build a standalone write-back tool; do not fork Renovate

## Context

Renovate is the gold standard for formatting-preserving, auto-merged PRs, so the obvious question is whether `rere` should fork or extend it rather than build its own write-back engine. The deciding factor is direction: right-sizing must frequently *lower* requests/limits (most savings are downsizing).

## Decision

We will **build `rere` as a standalone tool and not fork or extend Renovate's engine.** We will borrow only its *shapes*: the formatting-preserving in-place edit discipline, and its config vocabulary (`automerge`, `automergeStrategy`, `packageRules`, `groupName`, `schedule`). `rere` ships as a **CLI + GitHub Action first**; a **GitHub App mode** (install on a repo + a config file, the way Renovate is actually deployed) is future work.

## Alternatives considered

- **Fork/patch Renovate's engine** — rejected. Renovate's load-bearing invariant is "only ever move a value upward": `filterVersions` discards every candidate not strictly greater than current via `isGreaterThan`. The only downgrade path (`getRollbackUpdate`) is off-by-default and fires only when the current version vanishes from the registry — version recovery, not value reduction. Downsizing would require patching the core filter and maintaining a hostile fork against a ~biweekly-release codebase.
- **Custom versioning module for k8s quantities** — rejected. A *correct* one makes Renovate *better* at refusing downsizes; only a deliberately *lying* `isGreaterThan` would emit them.
- **`customDatasource` + custom/regex managers** — rejected as a foundation. Managers can *locate* `resources.requests.cpu`, and a custom datasource can return one synthetic "release," but both re-enter the strictly-greater filter and die on downsizes; the "registry of one version" is an awkward fit and the user-facing mental model is inverted.

## Consequences

- We own the full pipeline (adapter → discover → fieldmap → policy → yamledit → pr) — more code, but it fits the problem exactly (deadband both directions, headroom, limits policy, coupled edits).
- Renovate users still feel at home: we mirror `automerge`/`packageRules`/`groupName`/`schedule` and the App-or-Action deployment shape, and we adopt the same formatting-preserving edit discipline (realized via kyaml, [ADR-0001](0001-go-and-kyaml.md)).

## Confidence

High. The strictly-greater filter is Renovate's central invariant and the only downgrade path is narrow version recovery. We would reconsider only if Renovate added first-class bidirectional value updates. Informed by [RSCH-0002](../research/0002-renovate-internals.md) and [RSCH-0001](../research/0001-sota-prior-art.md).
