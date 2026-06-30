---
audience:
- operator-dev
created: '2026-06-30'
owners:
- Ca-moes
related:
  see_also: [ADR-0001]
status: active
tags: [krr, kubernetes, right-sizing]
title: KRR version support and compatibility
type: reference
updated: '2026-06-30'
visibility: public
---

# KRR version support and compatibility

rere's KRR adapter converts [robusta-dev/krr](https://github.com/robusta-dev/krr) `-f json` output into rere's internal `Target` vocabulary. This page records which KRR versions the adapter is verified against, the exact JSON wire shape it depends on, and the tolerant-parsing policy that lets one adapter build span multiple KRR releases. It is a lookup reference for contributors touching `internal/adapter`.

## Supported versions

| KRR version | Status | Notes |
|---|---|---|
| `v1.28.0` | Verified (pinned) | Wire shape read from `robusta_krr/core/models` at this tag; exercised by golden fixtures in `internal/adapter/testdata/krr/` and a captured real-cluster sample. |
| other `v1.x` | Best-effort | Parsed via the tolerant reader below; capture and verify a real sample before relying on it. |

The pinned reference version is **`v1.28.0`**. "Verified" means the field names and value encodings below were read from KRR's source at that tag and are covered by tests.

## Wire shape the adapter depends on

rere consumes only the `recommended` side of each scan. Current values are read from the manifest in the repo (via `yamledit.ReadCurrent`), not from KRR's cluster-side `object.allocations`.

| JSON path | Type | Mapped to |
|---|---|---|
| `scans[].object.namespace` | string | `Target.Namespace` |
| `scans[].object.kind` | string (`KindLiteral`) | `Target.Kind` — `GroupedJob` scans are skipped |
| `scans[].object.name` | string | `Target.Name` |
| `scans[].object.container` | string | `Target.Container` |
| `scans[].recommended.requests.{cpu,memory}` | value (see below) | `Recommended.Requests` |
| `scans[].recommended.limits.{cpu,memory}` | value (see below) | `Recommended.Limits` |

Each recommended value is `Union[RecommendationValue, Recommendation]`, where `RecommendationValue = float | "?" | null` and `Recommendation = {"value": RecommendationValue, "severity": string}`. CPU values are cores (`0.25` → `250m`); memory values are bytes (`134217728` → `128Mi`).

## Compatibility policy

The adapter parses defensively so one build tolerates minor KRR drift:

- Accepts both the wrapped `{value, severity}` object and a bare scalar for every recommended value.
- Maps `null`, `"?"`, and `"unset"` to "no recommendation" — the field is left untouched downstream.
- Skips (debug-logged), rather than errors on, `GroupedJob` scans and scans with no usable recommendation.
- Ignores unknown extra fields.

A scan is dropped only when it yields zero usable values. Malformed JSON is a hard error.

## Refreshing for a new KRR version

1. Pin the new tag and re-read `robusta_krr/core/models/{result,objects,allocations}.py` for shape changes.
2. Capture a real sample — `krr simple -f json > internal/adapter/testdata/krr/real-sample.json` against a representative cluster — then sanitize cluster and namespace names.
3. Run `go test ./internal/adapter/` and update the matrix above.
