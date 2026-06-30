---
name: docs-new
description: Scaffold a new doc under docs/ with correct frontmatter, the next-available ID, and the appropriate template. Use when the user wants to create a new ADR, research doc, how-to, tutorial, reference, explanation, or runbook.
argument-hint: <type> "<title>" [--runbook]
allowed-tools: Bash, Read
---

# docs-new

Scaffold a new doc in the current repo's `docs/` tree by invoking the `docs-framework new` CLI.

## Steps

1. Parse the user's arguments into `<type>` and `<title>`. Valid types:
   `tutorial`, `how-to`, `reference`, `explanation`, `adr`, `research`.
   Add `--runbook` for an operational how-to variant.
2. Invoke:

   ```bash
   docs-framework new <type> "<title>" [--owner <handle>] [--audience <csv>] [--visibility <value>] [--runbook]
   ```

   Prefer the installed `docs-framework` binary. If it's not on PATH, fall back to `uvx --from git+https://github.com/promptlylabs/docs-framework@v0.7.1#subdirectory=tooling docs-framework ...` <!-- x-release-please-version --> or `cd tooling && uv run docs-framework ...` (inside the framework repo).
3. Read the file path the CLI prints on stdout.
4. Tell the user the file was created and summarise the next steps: fill in the body, validate, open a PR.
5. If the user's next turn is "write the content", switch to the `docs-writer` subagent for the actual prose — it holds the format rules and writing conventions.

## When to defer

- **Type is ambiguous.** Run `/docs-classify` first; don't guess.
- **User wants to skip the CLI.** Only if you cannot run commands. In that case copy the matching template from `templates/<type>.md`, fill frontmatter yourself, and pick the next ID by scanning existing files.

## References

- [CLI reference](../../docs/reference/cli.md)
- [Frontmatter schema](../../docs/reference/frontmatter-schema.md)
