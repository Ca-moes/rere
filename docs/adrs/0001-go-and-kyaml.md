---
id: ADR-0001
type: adr
title: Go + kyaml for comment-preserving YAML edits
status: accepted
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev]
tags: [yaml, kubernetes]
related:
  implements: []
  informed_by: [RSCH-0003]
  supersedes: []
  superseded_by: []
  see_also: [ADR-0005]
---

# ADR-0001 — Go + kyaml for comment-preserving YAML edits

## Context

`rere`'s quality bar is surgical, minimal-diff edits to Kubernetes manifests that preserve comments, anchors/aliases, key order, and multi-document streams — never a whole-document re-marshal. The language and YAML library are sticky, foundational choices. The surrounding ecosystem we must integrate with (Flux API types, controller-runtime, client-go, go-github) is overwhelmingly Go.

## Decision

We will write `rere` in **Go**, and edit YAML with **`sigs.k8s.io/kustomize/kyaml`** (v0.21.1) — mutating individual scalar nodes in the `RNode` tree via `Lookup`/`LookupCreate`/`SetField`, read/written with `kio.ByteReader`/`ByteWriter`. We will never round-trip a whole document through a struct or `String()`.

## Alternatives considered

- **Rust** — rejected: no comment-preserving YAML round-trip library, and far from the k8s ecosystem (we'd reimplement Flux/k8s API types).
- **`gopkg.in/yaml.v3` node API** — rejected: docs state textual representation is not preserved on re-encode; comments drift; repo archived April 2025. ([RSCH-0003](../research/0003-yaml-editing-in-go.md))
- **goccy/go-yaml** — rejected for now: reversible edits require manual `CommentMap` handling and its path syntax lacks the k8s `[name=]` list idiom.
- **Shelling out to `yq`** — rejected: external-binary dependency, version skew, same go-yaml comment-drift caveats; poor fit for a single static binary.

## Consequences

- We inherit kyaml's gotchas: set `OmitReaderAnnotations: true`, consider `PreserveSeqIndent: true`, and never whole-doc re-marshal. These are encoded in the editor package and enforced by **golden-file tests**.
- kyaml wraps `yaml.v3`, so pathological comment placements can still shift — golden tests are the guard.
- Heavier dependency tree than a single YAML lib, accepted for fidelity + ecosystem fit.

## Confidence

High. kyaml is the only purpose-built comment/order/anchor-preserving k8s YAML editor and is the canonical ecosystem choice. We would reconsider only if golden-file fidelity proved unworkable for real manifests (anchors/multi-doc edge cases) — tracked as hardening work. Informed by [RSCH-0003](../research/0003-yaml-editing-in-go.md).
