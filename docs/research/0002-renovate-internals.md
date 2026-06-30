---
id: RSCH-0002
type: research
title: Renovate internals — can we reuse it for resource right-sizing?
status: completed
created: 2026-06-30
updated: 2026-06-30
owners: [Ca-moes]
visibility: public
audience: [operator-dev]
tags: [renovate, right-sizing]
related:
  implements: []
  informed_by: []
  supersedes: []
  superseded_by: []
  see_also: [ADR-0002]
---

# RSCH-0002 — Renovate internals: can we reuse it for resource right-sizing?

## Question

Renovate is the gold standard for formatting-preserving, auto-merged dependency PRs. Should `rere` fork or extend Renovate's engine, or build standalone and merely borrow its shapes? The crux: right-sizing must frequently *lower* requests/limits, and we need to know whether Renovate can structurally do that.

## Findings

### 1. Can Renovate emit a downgrade (a value strictly lower than current)? — Effectively no

The core update pipeline hard-filters candidates to strictly-greater-than-current. In `filterVersions` ([`lib/workers/repository/process/lookup/filter.ts`](https://github.com/renovatebot/renovate/blob/main/lib/workers/repository/process/lookup/filter.ts)):

```ts
let filteredReleases = versionedReleases.filter((r) =>
  versioningApi.isGreaterThan(r.version, currentVersion),
);
```

Every downstream filter only *narrows* that set. The **one** downgrade path is `getRollbackUpdate` ([`lookup/rollback.ts`](https://github.com/renovatebot/renovate/blob/main/lib/workers/repository/process/lookup/rollback.ts)), invoked from [`lookup/index.ts`](https://github.com/renovatebot/renovate/blob/main/lib/workers/repository/process/lookup/index.ts) **only** when `config.rollbackPrs && !allSatisfyingVersions.length` — i.e. the current version no longer exists in the registry. It is **off by default** (`rollbackPrs: false`) and is recovery from a yanked/missing release, not a general "this value should be lower" mechanism. **This is the disqualifying finding.**

### 2. Versioning modules — a custom one doesn't help

The `VersioningApi` interface ([`versioning/types.ts`](https://github.com/renovatebot/renovate/blob/main/lib/modules/versioning/types.ts)) is implementable; parsing `250m`/`512Mi`/`1Gi`/`1.5` into a comparable scalar is easy. **But** `filterVersions` calls *your* `isGreaterThan` to discard everything `≤ current`, so a *correct* quantity-versioning module makes Renovate **better at refusing to downsize**. The only way around it is a deliberately *lying* `isGreaterThan` — fighting the framework's core invariant.

### 3. Datasource model fit — poor

The model is "package + datasource → list of releases," each release with a required `version: string` ([`datasource/types.ts`](https://github.com/renovatebot/renovate/blob/main/lib/modules/datasource/types.ts)). Our input is a *single computed scalar* per (workload, container, resource), not an enumerable registry. [`customDatasource`](https://docs.renovatebot.com/modules/datasource/custom/) can be bent to return one synthetic "release," but it then re-enters the §1 filter and dies on any downsize.

### 4. Custom/regex/JSONata managers — locate yes, decide no

[`customManagers`](https://docs.renovatebot.com/configuration-options/#custommanagers) (regex / JSONata) can *locate* `resources.requests.cpu` via `matchStrings`/`depNameTemplate`/`currentValueTemplate`. **But** extracted deps still flow through the datasource-lookup + versioning pipeline — managers locate values, they don't choose them. Locating is solved; choosing a lower value is still blocked by §1.

### 5. Worth borrowing even without forking

- **Surgical in-place editing.** [`auto-replace.ts`](https://github.com/renovatebot/renovate/blob/main/lib/workers/repository/update/branch/auto-replace.ts) does `indexOf(replaceString)` then a substring splice into the original text — it does **not** re-parse/re-serialize, so comments/whitespace/order survive. This is exactly the editing *discipline* `rere` needs (we apply it via kyaml's node tree — see [RSCH-0003](0003-yaml-editing-in-go.md)).
- **Config vocabulary** ([options](https://docs.renovatebot.com/configuration-options/)): `automerge`, `automergeStrategy`, `packageRules`, `groupName`, `schedule`. Familiar prior art to mirror.

### 6. Deployment model

Primarily the **Mend-hosted Renovate GitHub App** (install + a `renovate.json` config), also runnable as a self-hosted GitHub **Action**, npm **CLI**, and Docker image ([running Renovate](https://docs.renovatebot.com/getting-started/running/)). This shapes `rere`'s own roadmap: CLI + Action first, a GitHub App mode later.

## Verdict: rebuild standalone, borrow patterns

Renovate's load-bearing invariant — only ever move a value **upward** — is the precise opposite of right-sizing. The only downgrade path is off-by-default version recovery. Making Renovate downsize needs either a lying versioning module or patching its core filter, i.e. a hostile fork against a fast-moving (~biweekly) codebase, plus an awkward one-version "registry" and an inverted mental model for users. **Build `rere` standalone**, explicitly stealing (1) the `auto-replace.ts` formatting-preserving edit discipline and (2) the `automerge`/`packageRules`/`groupName`/`schedule` config vocabulary + App-or-Action deployment shape. Backs [ADR-0002](../adrs/0002-standalone-not-renovate-fork.md).

> Note: line numbers drift on `main`; the stable anchors are the function names (`filterVersions`, `getRollbackUpdate`, `auto-replace`) and the `isGreaterThan` filter.
