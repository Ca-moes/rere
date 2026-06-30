---
id: ADR-0006
type: adr
title: Proprietary, all-rights-reserved license for now
status: accepted
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
  see_also: []
---

# ADR-0006 — Proprietary, all-rights-reserved license for now

## Context

`rere` fills a gap that is otherwise empty on the market ([RSCH-0001](../research/0001-sota-prior-art.md)). While the project is new we want the source to be **publicly viewable** (for transparency and evaluation) without letting others freely reuse, repackage, or build competing offerings on it. We need a license that reflects that intent.

## Decision

We will ship `rere` under a **proprietary, all-rights-reserved** `LICENSE`: the source is published for transparency/evaluation only, and no rights to use, copy, modify, or redistribute are granted without prior written permission. The `LICENSE` file lives on `main`; the README and docs describe the project as **source-available**, not open source. We explicitly intend to **relax to a more permissive license once the project matures**.

## Alternatives considered

- **MIT / Apache-2.0 (permissive OSS)** — rejected for now: lets anyone reuse and repackage a brand-new, unproven, market-gap product with no friction. Easy to adopt later.
- **FSL-1.1-MIT / BUSL-1.1 (source-available, time-delayed conversion)** — attractive ("free later" is built in), but they still grant substantial present-day use; we wanted maximum lock-down while the design stabilizes.
- **PolyForm Noncommercial** — allows all noncommercial use; broader than we want right now.

## Consequences

- No outside contributions or reuse without explicit permission; the audience is "look, evaluate, talk to us," not "fork and ship."
- GitHub will report no detected license (it only auto-detects OSI licenses) — expected and acceptable.
- Loosening later is a one-way, low-risk change (permissive → permissive is easy; the reverse is not), so starting restrictive preserves optionality. A future ADR will supersede this one when we relax the terms.

## Confidence

High for the near term. The main cost is foregone community contributions, which is acceptable for an early, single-author, market-gap project. We will revisit (and supersede this ADR) once the project is stable enough to benefit from broader adoption.
