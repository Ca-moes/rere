---
id: RSCH-0001
type: research
title: State of the art — Kubernetes right-sizing recommenders and write-back
status: completed
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev, platform-engineer]
tags: [overview, right-sizing, cost]
related:
  implements: []
  informed_by: []
  supersedes: []
  superseded_by: []
  see_also: [ADR-0002]
---

# RSCH-0001 — State of the art: Kubernetes right-sizing recommenders and write-back

## Question

Kubernetes right-sizing splits into two jobs: **computing** correct CPU/memory requests & limits, and **applying** those numbers back to a GitOps source of truth as reviewable, auto-merged PRs. Where does existing tooling sit, and is there a real gap for `rere` (the write-back half)?

## Findings

### Recommenders (compute the numbers — crowded, mature)

Almost none write the numbers back to a repo.

- **Robusta KRR** ([repo](https://github.com/robusta-dev/krr)) — agentless CLI that queries Prometheus directly. Default "simple" strategy: CPU request = p95 usage, memory request = max + 15%, **CPU limit unset**. Output is a table/JSON/CSV/HTML diff; it explicitly **does not write back** — you review the diff and roll it into GitOps yourself. Pure recommender.
- **Kubernetes VPA (recommender mode)** ([docs](https://kubernetes.io/docs/concepts/workloads/autoscaling/vertical-pod-autoscale/)) — in `Off` mode the recommender writes target/lowerBound/upperBound to the VPA object's `status`. The numbers live in cluster state, not your repo.
- **Fairwinds Goldilocks** ([repo](https://github.com/FairwindsOps/goldilocks)) — auto-creates `Mode: Off` VPAs and renders recommendations in a dashboard. A visualization layer over VPA; no write-back.
- **Kubecost / OpenCost** ([docs](https://docs.kubecost.com/using-kubecost/navigating-the-kubecost-ui/savings/container-request-right-sizing-recommendations)) — request right-sizing recommendations plus "1-Click" and scheduled auto-resizing that **edit live cluster requests directly** (Cluster Controller). Recommender + live applier, **not GitOps** — changes drift from Git.
- **Commercial** — StormForge Optimize Live, Cast AI Workload Autoscaler, PerfectScale, Densify. ML recommenders that primarily apply via **in-place pod resizing or mutating admission webhooks** (live, cluster-side), again creating Git drift. StormForge markets GitOps fit, but the default apply path is still a webhook.

### Write-back / prior art (apply the numbers — almost empty)

- **`aws-samples/K8sResourceResizer`** ([repo](https://github.com/aws-samples/K8sResourceResizer), [AWS blog](https://aws.amazon.com/blogs/containers/kubernetes-right-sizing-with-metrics-driven-gitops-automation/)) — the closest prior art. Source review (`Src/manifest_updater.py`, `manifest_finder.py`, `pr_opener.py`) **confirmed and refined** the brief's claims:
  - **ArgoCD-only** — only acts on deployments in an ArgoCD app's `status.resources`; no Flux support.
  - **Naive line-based string splicing** — rewrites `line.split('cpu:')[0] + f"cpu: {value}\n"`; no YAML serializer for writing. **Inline comments on edited lines are destroyed** (the docstring's "preserves file structure" is overstated).
  - **First container only** — `next(iter(limits.values()))`; multi-container pods silently lose all but the first.
  - **Brittle section matching** — triggers on any substring `resources:` with no indentation/container anchoring; can mis-target Kustomize/Argo `resources:` lists.
  - **No real Helm nesting / no operator CRs** — only top-level `resources` in `values.yaml`, only `kind: Deployment`.
  - **No auto-merge** — only `create_pull`; a human must review and merge.
- **VPA auto mode** — `Recreate`/`InPlace` apply to running pods. Cluster mutation, the opposite of GitOps write-back; leaves manifests stale.
- **GitOps-native updaters (image-only)** — Renovate ([automerge docs](https://docs.renovatebot.com/key-concepts/automerge/)) updates *dependencies*; Argo CD Image Updater writes a separate `.argocd-source-*.yaml` override; Flux Image Automation edits YAML only at inline `# {"$imagepolicy": ...}` markers. All **image-only** — none generalize to the many CPU/memory fields across containers, Helm values, and CRs.

## The gap `rere` fills

Every recommender ends with "now go edit your repo," and the only GitOps-native write-back tooling is image-specific. K8sResourceResizer is the lone resource-write-back attempt and is a narrow demo. The commercial/live appliers deliberately bypass Git, producing drift.

`rere` is the missing **write-back half**: structure-aware (comment/format-preserving) YAML edits, all containers, raw manifests + Helm values (incl. nested) + operator CRs, GitOps-native auto-merged PRs in the Renovate shape, recommender-agnostic input. Positioning: **"keep your recommender (KRR, VPA, Goldilocks, Kubecost, StormForge) — `rere` adds the write-back half."** This evidence backs [ADR-0002](../adrs/0002-standalone-not-renovate-fork.md) and is distilled in [explanation/prior-art.md](../explanation/prior-art.md).
