---
name: docs-link-check
description: Walk the generated index.json and flag docs that should cite each other but don't, plus broken cross-references. Use to find orphan docs or drift in the `related:` graph.
allowed-tools: Bash, Read
---

# docs-link-check

Generate the docs index and check the relationship graph for gaps and dangling refs.

## Steps

1. Generate the index:

   ```bash
   docs-framework index docs/ --out /tmp/docs-index.json
   ```

2. Run the strict validator — it reports authoritative broken references:

   ```bash
   docs-framework validate docs/
   ```

3. Read the index. For each entry:
   - **Broken refs** — any ID or path in `related.*` that the validator already flagged. Must fix.
   - **Orphans** — docs with no inbound edges *and* no outbound edges. Tutorials are naturally entry points, so exempt them unless their status is `deprecated`.
   - **Missing-link candidates** — pairs of docs sharing two or more tags that have no `related` edge between them. These are hypotheses, not errors.
4. Merge findings into a prioritised report:
   - Broken references first (blocking).
   - Orphans (review intent).
   - Link suggestions (optional).
5. Offer to fix the blocking items with Edit. For suggestions, let the user decide.

## Notes

- This skill is advisory for everything except broken references. Tag-overlap suggestions can produce false positives; treat them as a prompt for a human to judge.
- If `index.json` already exists in the repo (e.g. committed by the aggregator), use it instead of regenerating — it reflects the aggregator's post-filter view.
