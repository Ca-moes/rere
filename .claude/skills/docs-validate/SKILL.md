---
name: docs-validate
description: Run the docs-framework validator (and Vale prose linter) on the current repo's docs/ tree and report errors. Use after creating or editing docs to confirm changes pass CI.
allowed-tools: Bash
---

# docs-validate

Run two checks in order: the Python validator (hard gate), then Vale (advisory prose).

## Steps

1. From the repo root, run the structural validator:

   ```bash
   docs-framework validate docs/
   ```

2. If exit code is `0`, continue to step 4. If non-zero, surface each error (format: `severity: file:line: message`). Suggest the most likely fix based on the message:
   - `missing required field: X` → point to the [frontmatter schema](../../docs/reference/frontmatter-schema.md).
   - `unknown status / audience / tag` → the value isn't in the pinned taxonomy. Either typo, or add to `docs/tags-extra.yaml` for repo-specific tags.
   - `dangling reference` → `related.*` holds an ID or path that doesn't resolve.
   - `type X must live under docs/Y/` → the file is in the wrong folder for its declared type.
   - `framework pin … is a major version behind` → breaking changes upstream; review and bump `docs/.framework-version`.
3. Ask whether to apply the fixes, then use Edit / Write as appropriate.
4. Run the prose linter (if `vale` is on PATH):

   ```bash
   vale --minAlertLevel=warning docs/
   ```

   - Surface warning-level findings only (proselint hits, Weasel, ThereIs). Skip notice-level to avoid noise.
   - CI gates on Vale findings against added/modified lines. Fix any finding the run reports before pushing.
   - If `vale` isn't installed, note that CI will run it anyway and surface the same findings on the PR.

## Flags

- `--warnings-as-errors` on `docs-framework validate` for strict mode (fails on staleness too).
- `--json` on `docs-framework validate` for machine-readable output when scripting.

## Fallbacks

If `docs-framework` isn't on PATH:
- Inside the framework repo itself: `cd tooling && uv run docs-framework validate ../docs/`.
- In a consuming repo: `uvx --from git+https://github.com/promptlylabs/docs-framework@v0.7.1#subdirectory=tooling docs-framework validate docs/`. <!-- x-release-please-version -->

If `vale` isn't on PATH, install via [the Vale docs](https://vale.sh/docs/vale-cli/installation/) or skip — CI will surface the same findings on the PR.
