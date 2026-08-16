# Contributing

## Ground rules

**Zero dependencies.** The core and the shipped packs stay on the standard
library. This is why the pack format is JSON rather than YAML, and it is checked
in CI (`make deps`). A change that adds a `require` entry needs a case made for
it first.

**Determinism is a contract, not a nice-to-have.** Seeded output is stable for a
given algorithm version and pack fingerprint. Two rules follow:

- Never let a decision that reaches the output depend on Go map iteration
  order, on floating point, or on the order words appear in a source file.
- Never consume anything from the randomness source other than `Uint64()`. The
  reduction into a range is implemented in `internal/random` on purpose — using
  `rand.Rand.Uint64N` would tie our output to a standard library implementation
  detail, and a toolchain upgrade could then change every seeded name with
  nothing in this repository having changed.

**The engine knows nothing about products.** No database, no HTTP, no telemetry
SDK, no product-specific branching. A new theme is a pack, never a code change.

## Workflow

Before opening a pull request:

```
go build ./... && go vet ./... && golangci-lint run ./... && go test -race ./...
make deps
```

Add a `CHANGELOG.md` entry under **Unreleased**.

## Changing a vocabulary

Editing a `.txt` under `packs/` changes the pack fingerprint, which re-rolls
every seeded name any consumer has generated. That is expected and allowed — it
is exactly what the fingerprint exists to make visible — but it must be
deliberate:

1. Make the edit.
2. `go test -run Golden -update ./...`
3. Read the diff. The golden files carry the fingerprint on its own line; if it
   moved, seeded output moved with it.
4. Record the change in `CHANGELOG.md` under **Changed**, with the new
   fingerprint.

If the golden names changed but the fingerprint did *not*, something is wrong:
the vocabulary is unchanged, so the selection algorithm must have moved. That
breaks reproducibility without giving consumers any signal. Either revert, or
bump `algorithmVersion` in `fingerprint.go`.

## Curating words

The lists are curated by hand and the rules are enforced by review, not by code:

- 3 to 9 characters. Shorter reads as an abbreviation; longer overflows the
  length limits products actually configure.
- Lowercase `a-z` only, no accents. Localized packs are a future addition with
  their own files.
- Evocative but neutral: nothing with a political, religious, medical or
  otherwise loaded reading, and no trademarks.
- No duplicates. The loader rejects them within a category, so a mistake here
  fails the build rather than doubling a word's odds in silence.
- Concrete over abstract. "gaffer" and "lantern" carry a name; "thing" and
  "item" do not.

After a substantial edit, look at the output:

```
go test -run ReadableSample -v ./...
```

The filters exist for unlucky joins, not to compensate for a vocabulary. If
`TestShippedVocabularyIsClean` starts reporting a high filtered rate, the fix is
in the word list.

## Commits and versioning

Tags are created by semantic-release from the commit history, on every push to
`main` that passes CI. No GitHub Release is produced — `@semantic-release/github`
is deliberately absent from `.releaserc.json`, and tagging is core behaviour
rather than a plugin's.

Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Effect |
|---|---|
| `feat:` | minor |
| `fix:`, `perf:`, `refactor:` | patch |
| **any `(packs)` scope** | **minor** |
| `docs:`, `test:`, `chore:`, `ci:`, `style:` | no release |
| `BREAKING CHANGE:` footer | major — read the warning below first |

**Use the `packs` scope only when the fingerprint actually moves.** A vocabulary
change re-rolls every seeded name any consumer has generated, which is
observable behaviour even though the API is untouched, so it earns a minor bump
rather than a patch. A comment or a filename edit under `packs/` does not move
the fingerprint and should not carry the scope.

### Two Go-specific traps

**A `BREAKING CHANGE:` footer takes 0.x straight to 1.0.0.** semver treats it
that way and semantic-release obeys. While the vocabulary is still awaiting a
human review pass, staying in 0.x is the honest signal — so express breaking
changes as `feat:` and describe them in the changelog until you genuinely mean
to declare 1.0.

**Past v1, a major bump requires changing the module path.** Go encodes the
major version in the import path from v2 onward, so releasing `v2.0.0` means
`module github.com/improbabilitea/names-go/v2` in `go.mod` and a matching import
path everywhere. semantic-release will tag `v2.0.0` without knowing any of this;
the go.mod edit is on you, in the same commit.

### Tags are permanent

The Go module proxy caches tags immutably. Once anyone fetches a version it
exists forever, with whatever vocabulary it shipped. There is no unpublishing —
only moving forward.

## Tests

Standard library only — no testify, no go-cmp. Name tests for the scenario they
cover (`TestGenerate_SeedIsStableAcrossRetries`), and say in a comment *why* the
test exists when that is not obvious from the name. Several tests here exist
because a specific mistake is easy to make; that reasoning is worth more than
the assertion.

Use `fstest.MapFS` for pack fixtures rather than files under `testdata`, which
is reserved for golden output.
