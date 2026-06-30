---
id: RSCH-0003
type: research
title: Comment-preserving YAML editing in Go
status: completed
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev]
tags: [yaml, kubernetes]
related:
  implements: []
  informed_by: []
  supersedes: []
  superseded_by: []
  see_also: [ADR-0001]
---

# RSCH-0003 — Comment-preserving YAML editing in Go

## Question

`rere` must mutate `resources.requests`/`limits` scalar values inside existing manifests while preserving comments, anchors/aliases, key order, and multi-document streams, with a **minimal diff** — never a whole-doc re-marshal. Which Go library?

## Findings

| Capability | kyaml (`RNode`) | `yaml.v3` `Node` | goccy/go-yaml | yq (shell-out) |
|---|---|---|---|---|
| Comment preservation | Yes | Partial — drifts | Via `CommentMap` (manual) | Best-effort, drifts |
| Anchor/alias preservation | Yes (`DeAnchor` opt-in) | Node-level only | Yes (claimed) | Yes |
| Key-order preservation | Yes | Yes (node API) | Yes | Yes |
| Multi-doc streams | Yes (`kio`) | Manual loop | Yes | Yes |
| Minimal diff | Strong (scalar mutation) | Weak (re-encode reflows) | Moderate | Moderate |
| List-element-by-name nav | Yes — `[name=foo]` | Hand-rolled | No k8s idiom | jq-style |
| k8s ecosystem fit | Native | Generic | Generic | External binary |

### kyaml (`sigs.k8s.io/kustomize/kyaml`)

Current version **v0.21.1** (2026-02-09, [pkg.go.dev versions](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml?tab=versions)). `RNode` wraps `*gopkg.in/yaml.v3.Node` (via `YNode()`); you mutate individual scalar nodes rather than re-marshaling, which is what yields minimal diffs ([kyaml/yaml docs](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml/yaml)). `Lookup`/`LookupCreate` accept the `[fieldName=fieldValue]` element matcher, e.g. `Lookup("spec","template","spec","containers","[name=app]","resources","limits","cpu")` ([fns.go](https://github.com/kubernetes-sigs/kustomize/blob/master/kyaml/yaml/fns.go)). `FieldSetter` applies a style only if none exists or `OverrideStyle` is set (default false), so existing quoting is retained. `DeAnchor` is opt-in, so anchors survive by default.

**Gotchas (verified):** `kio.ByteReader` injects `config.kubernetes.io/index` / `internal.config.kubernetes.io/seqindent` annotations — set `OmitReaderAnnotations: true` (or rely on `ByteWriter` stripping them) or they pollute output. Use `PreserveSeqIndent: true` to keep original list indentation. **Never** round-trip a whole doc through `String()`/structs — mutate nodes in place ([kio docs](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml/kio)).

### Alternatives

- **`gopkg.in/yaml.v3` `Node`** — docs state re-encoded content "will **not** have its original textual representation preserved" ([pkg.go.dev](https://pkg.go.dev/gopkg.in/yaml.v3)); comments reattach to the last child ([#709](https://github.com/go-yaml/yaml/issues/709)), blank lines don't round-trip ([#627](https://github.com/go-yaml/yaml/issues/627)), and the repo was **archived April 2025**. You'd hand-roll what kyaml already gives you.
- **goccy/go-yaml** — actively maintained; reversible transforms via a `CommentMap` you capture on decode and re-apply via `WithComment` ([README](https://github.com/goccy/go-yaml/blob/master/README.md)) — workable but manual, and its YAMLPath lacks the k8s `[name=]` idiom.
- **yq (shell-out)** — preserves comments/anchors and edits in place, but is built on the same go-yaml family (same comment-drift caveats) and adds an external-binary dependency, version-skew risk, and subprocess handling — a poor fit for a single-binary Go tool.

## Verdict

**Use kyaml** (`sigs.k8s.io/kustomize/kyaml` v0.21.1). Purpose-built for in-place, comment/anchor/order-preserving, minimal-diff edits of Kubernetes config, with first-class `[name=foo]` list navigation and native ecosystem fit. **Honest weaknesses:** it inherits `yaml.v3`'s comment-attachment quirks (pathological placements can still shift — hence golden-file tests), a heavier dependency tree, and the annotation/seq-indent gotchas above must be handled explicitly. Backs [ADR-0001](../adrs/0001-go-and-kyaml.md).

```go
import (
    "sigs.k8s.io/kustomize/kyaml/yaml" // RNode, Lookup, LookupCreate, SetField, FieldSetter, NewScalarRNode
    "sigs.k8s.io/kustomize/kyaml/kio"  // ByteReader, ByteWriter
)
// require sigs.k8s.io/kustomize/kyaml v0.21.1
```
