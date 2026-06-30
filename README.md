# rere

> **ré-ré** = **re**source **re**sizer

**The missing write-back half of Kubernetes resource right-sizing.**

Recommenders (Robusta [KRR](https://github.com/robusta-dev/krr), VPA, Goldilocks) already tell you the
right CPU/memory requests & limits by reading Prometheus history. The gap nobody fills is **safely
writing those numbers back into a GitOps repo** — across raw manifests, Helm `values:`, *and* operator
Custom Resources — as **clean, auto-merged pull requests**, with surgical edits that preserve your
comments and formatting.

`rere` is a Go CLI + GitHub Action that does exactly that. It is **complementary to
your recommender, not a replacement**:

> **Keep your recommender — `rere` adds the write-back half.**

## How it works

```
KRR JSON → adapter → discover (owning Flux object + repo path) → fieldmap (resolve field)
        → policy (deadband / headroom / limits) → yamledit (surgical edit) → PR + auto-merge
```

Unlike a naive find-and-replace, `rere` edits YAML through a structure-aware tree (kyaml), so comments,
anchors, and key order survive — the diff shows only the numbers that changed. And unlike dependency
bots, it can move values **down**: most right-sizing savings come from *downsizing*.

## Status

🚧 Early development. The v0.1 (MVP) milestone targets Flux + GitHub, the three field-map tiers, the KRR
adapter, deadband policy, and auto-merge. Follow progress in the
[issues](https://github.com/Ca-moes/rere/issues).

## Documentation

Design, architecture, and the decisions behind them live under [`docs/`](docs/):

- [Architecture & implementation direction](docs/explanation/architecture.md)
- [Prior art and why rere exists](docs/explanation/prior-art.md)
- [Architecture Decision Records](docs/adrs/) · [Research](docs/research/)

## License

**Proprietary — all rights reserved.** The source is published for transparency and evaluation only; it
is **not** open source and may not be used, copied, modified, or redistributed without prior written
permission. See [LICENSE](LICENSE). The intent is to relax this to a more permissive license once the
project matures.
