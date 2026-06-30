---
id: ADR-0003
type: adr
title: Discover target manifests by scanning the repo, not the cluster
status: accepted
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev, platform-engineer]
tags: [flux, gitops]
related:
  implements: []
  informed_by: []
  supersedes: []
  superseded_by: []
  see_also: [ADR-0004]
---

# ADR-0003 — Discover target manifests by scanning the repo, not the cluster

## Context

Given a recommendation for a workload (namespace/kind/name/container), `rere` must find which file in the GitOps repo holds that workload's manifest. There are two broad strategies: **scan the repo** (the way Renovate works — you point it at a repo and it reads the files), or **query the cluster** for Flux provenance (Flux records which `Kustomization`/`HelmRelease` owns each live object and where its source lives). Requiring a live cluster connection for the *write-back* step is a significant operational burden — a kubeconfig, network reachability, and Flux CRDs on every run — and departs from the Renovate model `rere` deliberately emulates.

## Decision

**`rere` discovers target manifests by scanning the configured GitOps repo (the local checkout from [ADR-0004](0004-local-checkout-github-api-write.md)) and matching each recommendation's `kind` + `name` (+ `namespace` when present) to a manifest document. No cluster connection is required** — the same operational model as Renovate. Ambiguity (the same workload appearing in multiple files/overlays) is handled by pointing `rere` at a scoped repo/path and, where needed, include/exclude path config.

**Cluster-based Flux provenance discovery is deferred** to a future, optional enhancement: when a cluster is reachable, reading the Flux provenance labels (`kustomize.toolkit.fluxcd.io/*`, `helm.toolkit.fluxcd.io/*`) resolves the exact owning object + path unambiguously. It is an add-on for hard cases, never a requirement.

## Alternatives considered

- **Cluster-provenance first (require a cluster)** — rejected for v1. It is unambiguous, but forces a kubeconfig + reachable cluster + Flux CRDs on every write-back run, adds heavyweight deps (controller-runtime, client-go, the Flux APIs), and breaks the "point it at a repo" model. Kept as deferred, optional disambiguation.
- **Repo-scan with no disambiguation story** — rejected: in repos with multiple overlays/environments the same `name` can appear more than once. We accept repo-scan's ambiguity but mitigate it with repo/path scoping (and, later, optional provenance).

## Consequences

- **Much simpler v1**: no cluster libraries, kubeconfig handling, or Flux CRD dependence in the M1 build; `rere` runs anywhere the repo is checked out (laptop, CI, Action).
- **Ambiguity is the accepted tradeoff**: when a workload matches multiple manifests, `rere` relies on the configured scope (which repo/path it was pointed at) and path filters; truly ambiguous matches are flagged rather than guessed. Optional provenance later removes the ambiguity for those who have a cluster.
- **Namespace caveat**: raw manifests sometimes omit `metadata.namespace` (it's injected by Kustomize/Flux). Matching is on `kind` + `name` within the configured scope, using `namespace` only when the manifest carries it.

## Confidence

High. Repo-scan is exactly how Renovate operates at scale, so the model is proven; the ambiguity tradeoff is real but bounded by scoping and is the same tradeoff every repo-scanning tool accepts. We would add the optional provenance path if real-world repos prove too ambiguous to resolve by scope alone. Relates to [ADR-0004](0004-local-checkout-github-api-write.md).
