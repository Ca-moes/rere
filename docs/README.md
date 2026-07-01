# rere docs

`rere` (ré-ré = **re**source **re**sizer) turns Kubernetes resource right-sizing recommendations into auto-merged GitOps PRs, via surgical comment-preserving YAML edits. These docs follow the [Promptly docs-framework](https://github.com/promptlylabs/docs-framework) (pinned in [`.framework-version`](.framework-version)).

## Start here

- **"What is rere and how is it built?"** → [explanation/architecture.md](explanation/architecture.md)
- **"What else is out there and why build this?"** → [explanation/prior-art.md](explanation/prior-art.md)
- **"What did we decide, and why?"** → [adrs/](adrs/)
- **"What evidence backs those decisions?"** → [research/](research/)

## Layout

- [explanation/](explanation/) — architecture and rationale (the implementation direction).
- [adrs/](adrs/) — settled decisions with trade-offs (`ADR-NNNN`).
- [research/](research/) — investigations behind the decisions (`RSCH-NNNN`).
- [reference/](reference/) — lookup material (version/compatibility matrices, config).
- [how-to/](how-to/), [tutorials/](tutorials/) — fill in as the tool lands.

## Validating these docs

No CI gating (the framework's `docs-check` Action lives in a private repo we don't run from here). Validate locally:

```bash
uvx --from git+https://github.com/promptlylabs/docs-framework@v0.7.1#subdirectory=tooling \
  docs-framework validate docs/
```
