# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make test         # go test -race ./...
make lint         # golangci-lint run ./...
make bench        # go test -bench=. -benchmem ./...
make deps         # fails if go.mod acquires any external dependency
make test-fuzz    # short fuzz pass over the loader and over seeds
```

Pre-commit gate (from CONTRIBUTING.md) — all four must pass:

```bash
go build ./... && go vet ./... && golangci-lint run ./... && go test -race ./...
```

Single test, benchmark, fuzz:

```bash
go test -run TestGenerate_SeedIsStable ./...
go test -bench BenchmarkGenerate -benchmem -run NONE .
go test -run=NONE -fuzz=FuzzLoadFS -fuzztime=60s .
```

Golden files (see "Changing a vocabulary" below):

```bash
go test -run Golden -update ./...
```

## Architecture

A pure-computation library: no I/O, no state, no dependencies outside the standard
library. The reasoning behind each non-obvious choice lives in the code comments
next to it — those are the design record, so read them before changing the thing
they explain.

### The pipeline

```
pack.json + *.txt
  → LoadFS        parse, validate locally, sort canonically   (loader.go)
  → New           merge packs, compose weights, compile        (generator.go)
  → cursor        select → format → filter → retry → escalate  (generate.go)
```

`New` does everything expensive and everything fallible. A `Generator` is
immutable afterwards, so it is shared across goroutines without a lock and the
randomness source is created per call.

### Determinism is the load-bearing constraint

Seeded output is contractually stable for a given `algorithmVersion` (in
`fingerprint.go`) and `packFingerprint`. Four rules follow, and breaking any of
them breaks the contract silently:

1. **No Go map iteration on the output path.** Maps are for lookup and dedup
   only; every decision iterates a sorted slice (`sortedKeys` in `loader.go`).
2. **Never `rand.Rand.Uint64N`/`IntN`** — only the raw `PCG.Uint64()` stream.
   Range reduction lives in `internal/random` on purpose: the standard library's
   reduction is an implementation detail, and a toolchain upgrade changing it
   would move every seeded name with nothing in this repo having changed. This is
   why `Source` exposes only `Uint64()`, and why CI runs two Go versions.
3. **No floating point** in weighted selection (`internal/weighted`).
4. **Canonical sorting twice** — once per pack in `LoadFS`, once over the merged
   set in `New`. Both are required: the first enables adjacent-duplicate
   detection, the second describes a set no earlier sort has seen.

The fingerprint hashes content that reaches the output and deliberately excludes
what does not: line order in a `.txt`, JSON formatting, the declared pack version,
tags and locale. Disabled words are skipped rather than hashed.

### Two-level validation

`LoadFS` validates a pack **in isolation**; `New` validates the **merged set**.
The split is necessary, not stylistic: a theme pack legitimately references
categories it does not define (`cinema` builds on `adjective` from `common`), so
it must load alone and fail only when registered without its base. That failure —
`ErrDictionaryNotFound` from `resolveReferences` — is the most likely integration
mistake, and the error names both the missing category and the pack that needs it.

### Weight composition

Template weights are relative to their own pack, normalized per pack, then scaled
by `PackWeight`. `compileTemplates` brings each pack's total to their least common
multiple so every share is an exact integer. The property this buys: adding a
template to a pack redistributes weight *within* that pack and takes none from the
others. There is a test for exactly that.

### Escalation ladder vs weighted mixture

The counter-intuitive fact the ladder exists for: two draws collide with
probability `Σ p²/space`, which is **not an average**. A template with a small
space and non-trivial weight dominates the sum, and no quantity of large-space
templates alongside it compensates. Weighting three-word templates more heavily
therefore does *not* buy uniqueness.

A ladder is not a mixture — it exhausts one rung before using the next, so the
spaces **add**. For the shipped packs that is the difference between 232k
effective names and 262M. `EstimateCardinality` reports the figure and, per rung,
which template is throttling it.

`GenerateUnique` and `Generate` share one `candidateCursor`, so a filter rejection
and a candidate the caller reports as taken advance the same counters and the same
rung.

## Traps

**Editing any `.txt` under `packs/` moves the fingerprint** and re-rolls every
seeded name. Golden tests will fail, and that is the design working. Regenerate
with `go test -run Golden -update ./...`, read the diff — the golden files carry
the fingerprint on its own line — and record it in `CHANGELOG.md`. If the golden
names changed but the fingerprint did *not*, the selection algorithm moved:
revert, or bump `algorithmVersion`.

**`GenerateN` deduplicates within the batch.** It is for offering alternatives.
To sample a distribution, loop `Generate()` instead — with replacement. Several
tests were written the wrong way round before this was fixed.

**`commonPack(t)` / `cinemaPack(t)` in `generator_test.go` are 5-word fixtures,
not the shipped packs.** The shipped packs cannot be imported from package `names`
(they import it), so tests touching them live in `packs_test.go` under
`names_test`.

**`unused` lint findings are expected mid-change** for symbols a later step will
consume. Lint must be clean before committing, not during.

## Releases

Pushes to `main` that pass CI are tagged by semantic-release (`.releaserc.json`,
`release` job in `ci.yml`). It creates **tags only** — no GitHub Release, because
`@semantic-release/github` is omitted on purpose.

Node exists solely for this: `package.json` is `private`, pinned, and locked with
pnpm. It never reaches `go.mod`, and the `deps` job still fails on any Go
dependency.

Commit prefixes decide the version — see CONTRIBUTING.md. Two traps documented
there and worth repeating: a `BREAKING CHANGE:` footer jumps 0.x to 1.0.0, and
any major past v1 requires changing the module path to `/vN`.

Local dry run:

```bash
pnpm install --frozen-lockfile
pnpm run release:dry
```

## Conventions

Standard library only in tests — no testify, no go-cmp. Tests are named for the
scenario they cover (`TestXxx_Scenario`), and errors are package-level sentinels
prefixed `names:`, matched with `errors.Is`.

Two deliberate departures: `NoValidCandidateError` and `PackValidationError` are
structured types, because they carry a payload a sentinel cannot — but both
`Unwrap` to a sentinel so `errors.Is` stays the primary idiom. And `testdata/` is
used, for golden files only; pack fixtures use `fstest.MapFS`.

Comments explain *why*, particularly where a choice is non-obvious or where a
specific mistake is easy to make.

## Vocabulary

Curation rules are in `CONTRIBUTING.md`: 3–9 characters, lowercase `a-z`,
concrete over abstract, no trademarks, no loaded connotations. The loader rejects
duplicates within a category, so that class of mistake fails the build.

**The word lists were generated, not sourced, and have not had a human review
pass.** Two cleanup rounds removed four trademarks (`dolbylab`, `finalcut`,
`moviola`, `steadicam`) and roughly twenty invented compounds that read as real
vocabulary but are not. Assume more remain. Bulk additions degrade quality fast —
measure with `EstimateCardinality` before growing a list, since the leverage is
almost never where it looks, and read the output afterwards:

```bash
go test -run ReadableSample -v ./...
```

## Other agent configs

A Codex config exists at `~/.codex/config.toml`. To import MCP servers, commands,
subagents or skills from it, reply `/import` to see what is importable, then
`/import --yes=<digest>` to apply. If `/import` is unavailable on this surface,
run `claude import` from a terminal.
