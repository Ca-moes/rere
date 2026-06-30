---
name: docs-classify
description: Given a draft doc or a description of content, classify it into the right Diátaxis type (tutorial, how-to, reference, explanation, ADR, or research) and recommend a folder. Use when the user has written content but isn't sure where it belongs.
allowed-tools: Read
---

# docs-classify

Apply the framework's mode-disambiguation rules to a draft and return a type recommendation.

## Steps

1. Get the content. If the user provided a file path, read it. Otherwise ask for a summary of what the draft is about and who it's for.
2. Apply these rules in order (stop at the first match):
   - **Teaches step-by-step from zero** to a working outcome → **tutorial** (`tutorials/`). Audience is a beginner.
   - Answers **"how do I X"** for someone who already knows the basics → **how-to** (`how-to/`). If it's for on-call incident response, add `tag: runbook`.
   - A **dry, structured catalogue** of fields, endpoints, flags, config → **reference** (`reference/`).
   - Explains **why** something works the way it does — architecture, trade-offs, mental models → **explanation** (`explanation/`).
   - Records a **point-in-time decision** with alternatives weighed → **ADR** (`adrs/`).
   - Records an **investigation** (may or may not lead to a decision) → **research** (`research/`).
3. If the draft spans multiple modes (common with legacy "guide" docs), recommend splitting it and describe where each part belongs.
4. Report the recommendation with one sentence of reasoning.
5. Offer to run `/docs-new <type> "<title>"` to scaffold the correct home, then help move the content.

## References

- [README § Content types](../../README.md#content-types) — the full type table.
- [docs/explanation/prior-art.md](../../docs/explanation/prior-art.md) — the reasoning behind this split (Diátaxis + ADR + Research).
