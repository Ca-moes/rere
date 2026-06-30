---
type: explanation
title: Prior art and why rere exists
status: active
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev, platform-engineer]
tags: [overview, right-sizing]
related:
  informed_by: [RSCH-0001]
  see_also: [ADR-0002]
---

# Prior art and why rere exists

## Summary

Kubernetes right-sizing splits into **computing** the numbers (a crowded, mature space) and **applying** them back to Git as reviewable, auto-merged PRs (almost empty). `rere` targets the second half. The full survey with citations is [RSCH-0001](../research/0001-sota-prior-art.md); this page is the distilled "why."

## Recommenders compute, they don't write back

KRR, VPA (recommender mode), and Goldilocks all *compute* requests/limits and then stop — KRR prints a diff you're meant to roll into GitOps yourself; VPA writes to an object's `status`; Goldilocks visualizes VPA output. Kubecost and the commercial tools (StormForge, Cast AI, PerfectScale) that *do* apply changes typically do so **live** — in-place pod resizing or mutating webhooks — which edits the cluster, not your repo, creating drift from Git. None of them produce GitOps PRs as their primary path.

## The write-back space is nearly empty

The only direct prior art is **`aws-samples/K8sResourceResizer`**, and reading its source confirms it is a narrow demo: ArgoCD-only; naive line-based string splicing that **destroys inline comments** on edited lines; first-container-only; brittle substring matching for `resources:`; no real Helm nesting and no operator CRs; and **no auto-merge**. The GitOps-native updaters that *do* preserve formatting — Renovate, Argo CD Image Updater, Flux Image Automation — are all **image-only** and don't generalize to the dozens of CPU/memory fields spread across containers, Helm values, and CRs.

## Why not just use Renovate?

Renovate is the right *shape* (formatting-preserving, auto-merged PRs) but the wrong *engine*: its lookup filters candidates to those strictly greater than current, so it structurally cannot emit the **downsizing** PRs where most savings live. The full analysis is [RSCH-0002](../research/0002-renovate-internals.md); the decision is [ADR-0002](../adrs/0002-standalone-not-renovate-fork.md). We borrow Renovate's config vocabulary (`automerge`, `packageRules`, `groupName`, `schedule`) and its in-place edit discipline, not its lookup.

## The gap rere fills

`rere` is the missing **write-back half**: structure-aware (comment/format-preserving) edits, all containers, raw manifests + Helm values (incl. nested) + operator CRs, GitOps-native auto-merged PRs, recommender-agnostic input. **Keep your recommender — `rere` adds the write-back half.**
