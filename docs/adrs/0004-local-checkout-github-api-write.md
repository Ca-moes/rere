---
id: ADR-0004
type: adr
title: Local-checkout read/edit + GitHub-API write
status: accepted
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev, platform-engineer]
tags: [gitops]
related:
  implements: []
  informed_by: []
  supersedes: []
  superseded_by: []
  see_also: [ADR-0003]
---

# ADR-0004 — Local-checkout read/edit + GitHub-API write

## Context

`rere` needs to read the target manifests, edit them, and open an auto-merged PR. There are several shapes: operate purely against the GitHub API (no clone), operate on a local checkout with `git push`, or a hybrid. The editor (kyaml) and the dry-run experience are much easier to test and demo against files on disk.

## Decision

**Read and edit from a local checkout** (`--repo ./path`, expected to be at the base ref), then **write via the GitHub API**: create a branch, build a tree/commit with the edited file contents using the Git Data API (atomic multi-file commits), open a PR, and enable auto-merge via the `enablePullRequestAutoMerge` GraphQL mutation. `--dry-run` prints a unified diff and returns before any write — needing no credentials.

## Alternatives considered

- **GitHub-API-only (no clone)** — rejected for v1: harder to test, and scanning a directory to locate the right manifest is awkward over the API. Attractive for an Action; revisit for the GitHub App mode.
- **Local git for everything (`git push`)** — rejected: needs push credentials/`go-git`, and we'd still use the API for the PR + auto-merge. Using the Git Data API for the write keeps auth to a single token and gives atomic multi-file commits.

## Consequences

- Auto-merge is GraphQL-only (no REST/go-github method); the mutation takes the PR **node_id**, not its number.
- Operational preconditions live outside `rere` and must be documented: the repo must enable "Allow auto-merge"; the base branch needs protection with a required check; the default `GITHUB_TOKEN` / GitHub Apps often can't trigger required checks → stuck PRs, so a PAT is recommended.
- The local checkout must be at the base ref so committed content matches what we read.

## Confidence

High for the mechanism (Git Data API + GraphQL auto-merge are well-trodden). The main user-facing risk is the auto-merge preconditions, mitigated with clear docs and actionable errors. Relates to [ADR-0003](0003-repo-scan-discovery.md).
