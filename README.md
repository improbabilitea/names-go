# names

`names` generates readable, memorable names — usernames, slugs, display names,
workspace and environment labels — from composable themed vocabularies. No
network, no database, no dependencies outside the standard library.

```
go get github.com/improbabilitea/names-go
```

The module is `github.com/improbabilitea/names-go`; the package is `names`. The
`-go` suffix is a repository convention and does not appear in code:

```go
import names "github.com/improbabilitea/names-go"
```

## Quick start

```go
generator, err := names.New(
    names.WithPack(common.Pack, names.PackWeight(1)),
    names.WithPack(cinema.Pack, names.PackWeight(4)),
    names.WithFormat(names.Username),
    names.WithMinLength(5),
    names.WithMaxLength(20),
    names.WithCharset(names.CharsetLowerAlnum),
    names.WithReservedWords(reserved.Default),
)
if err != nil {
    return err
}

result, err := generator.Generate()
// result.Value == "midnightframe"
```

Construction validates everything and is the expensive part; a `Generator` is
immutable afterwards and safe for concurrent use. Build one per process — a
package-level `MustNew` is the idiomatic shape — and share it.

Sample output from the shipped packs:

```
apronpika        cherryblocking   serenadestage    printwoodlark
backstageanhinga backlightcolobus jaspervividballyhoo
```

## Packages

| Package | Role |
|---|---|
| `names` | the engine: `New`, `Generate`, `LoadFS`, formats, filters, constraints |
| `packs/common` | the base vocabulary — adjectives, animals, colors, moods, nature, objects, space |
| `packs/cinema` | a theme pack, and the worked example of composing one |
| `filters/reserved` | default list of terms a product should keep for itself |
| `filters/profanity` | optional profanity filtering, kept out of the core |

Each pack is a separate Go package with its own `embed.FS`, so the linker
includes only what you import.

## Formats

One logical name, five renderings. The format is applied after selection, so the
same words can be shown differently in different places.

| Format | Output |
|---|---|
| `names.Username` | `midnightframe` |
| `names.Slug` | `midnight-frame` |
| `names.DisplayName` | `Midnight Frame` |
| `names.Snake` | `midnight_frame` |
| `names.Pascal` | `MidnightFrame` |

## Composing packs

A theme pack contributes its own nouns and builds on the base categories, so the
two are registered together:

```go
names.WithPack(common.Pack, names.PackWeight(1)),
names.WithPack(cinema.Pack, names.PackWeight(4)),
```

Template weights are relative to their own pack and are normalized before the
pack weight applies. The pack author controls the proportions inside a theme;
you control the balance between themes. At 1 and 4, four names in five use a
cinema template — and adding a template to `cinema` redistributes weight within
`cinema` rather than taking any from `common`.

Registering a theme pack alone fails at construction with
`ErrDictionaryNotFound`, naming the category it needs.

## Custom packs

A pack is a `pack.json` manifest plus one word per line in `.txt` files:

```json
{
  "name": "brewing",
  "dictionaries": { "brew_noun": "brew_nouns.txt" },
  "templates": [
    { "pattern": ["adjective", "brew_noun"], "weight": 3 },
    { "pattern": ["color", "brew_noun"], "weight": 1 }
  ]
}
```

`LoadFS` takes any `fs.FS`, so an `embed.FS` compiled into your binary and a
directory on disk load identically:

```go
//go:embed pack.json *.txt
var packFS embed.FS

pack, err := names.LoadFS(packFS)
pack, err := names.LoadFS(os.DirFS("/etc/myproduct/names"))
```

Nothing in the engine changes to accommodate a new pack.

## Determinism

A seed makes generation reproducible:

```go
generator.Generate(names.Seed("user_01893"))
```

The seed is hashed with the algorithm version and the pack fingerprint before it
reaches the generator, so it never appears in the output — passing an email or
an internal identifier leaks nothing — and consecutive seeds produce
uncorrelated names.

**Read this before relying on it.** The guarantee is precise and narrow:

> Seeded output is stable for a given **algorithm version** and **pack
> fingerprint**. It is *not* stable across pack versions.

Any change to a vocabulary moves the fingerprint and re-rolls every seeded name,
because weighted selection depends on the whole word set. Adding a single word
is enough. There is no middle ground, and this library does not pretend there
is: `Generator.Fingerprint()` exposes the value so you can detect the change
rather than discover it as silently different names.

```go
generator.Fingerprint() // "sha256:45b6ac86…"
```

The practical consequence: **if a name must be stable for years, persist the
generated name, not the seed.** Seeded generation is the right tool for
fixtures, tests, ephemeral environments and regenerable identifiers — not a
substitute for storage.

## Uniqueness

The library does not guarantee uniqueness and holds no state. Generate, insert,
and let a unique constraint arbitrate:

```
generate → INSERT → unique violation → generate another
```

Checking availability and then writing is a race. `GenerateUnique` exists for
the cases where a read-then-act is genuinely acceptable, and takes the check
from you so the engine stays ignorant of any datastore:

```go
result, err := generator.GenerateUnique(ctx,
    func(ctx context.Context, candidate string) (bool, error) {
        return repo.IsAvailable(ctx, candidate)
    })
```

### Asking for several names at once

`GenerateN` is the call for offering a choice, and the names it returns are
distinct from each other:

```go
results, err := generator.GenerateN(10)
```

Calling `Generate` ten times in a loop is not the same thing. Each call derives
its stream from the seed alone, so **ten seeded calls return the same name ten
times**. Unseeded it works, but you pay ten stream derivations instead of one.

Distinctness is within the batch only — it says nothing about names already in
your database. Asking for more names than the vocabulary can distinctly produce
fails with `ErrNoValidCandidate` rather than returning duplicates.

For sampling the distribution rather than collecting alternatives — checking how
often a theme comes through, say — loop `Generate` instead: that samples with
replacement, which is what a distribution needs.

### Uniqueness at scale: use the escalation ladder

If names must stay unique across a large user base, configure a ladder:

```go
names.WithEscalation(
    names.StepTemplateArity(2, names.Attempts(40)),  // velvetfox
    names.StepTemplateArity(3, names.Attempts(40)),  // crimsonvelvetfox
    names.StepNumericSuffix(3, names.Attempts(20)),  // smoothcut383
)
```

Rungs are tried in order. A filter rejection *and* a candidate your check
reports as taken both consume an attempt, so real collisions are what push the
generator down the rungs. Early users get short names; later ones get longer
ones; nobody gets a duplicate.

**This is not the same as weighting three-word templates more heavily, and the
difference is large.** Two independent draws collide with probability
`Σ p²/space` over the templates. That is not an average: a template with a small
space and non-trivial weight dominates the sum, and no quantity of large-space
templates alongside it compensates. A ladder is not a mixture — it exhausts one
rung before using the next, so the spaces *add* instead of averaging.

With the shipped packs:

| Configuration | Effective names | Users at 1% collisions |
|---|---:|---:|
| Weighted mixture | 232k | 2.3k |
| With the ladder | **262M** | **2.6M** |

### Sizing the vocabulary

`EstimateCardinality` turns "the vocabulary looks big" into a number, and says
which template is throttling it — which is rarely the one you would guess:

```go
fmt.Println(generator.EstimateCardinality())
```

```
effective 261.8M   upper 68.7G
users at 1% collisions: 2.6M   at 10%: 26.2M

2-word templates — effective 151.7k, upper 460.0k
! adjective+cinema_noun          p= 39.6%  space=  37.2k  collisions= 64.0%
  cinema_noun+animal             p= 19.8%  space=  31.4k  collisions= 19.0%
  ...

3-word templates — effective 29.3M, upper 68.1M
  ...
```

The `!` marks a template causing more than a quarter of its rung's collisions.
That is where a word list is worth growing; everywhere else is close to wasted
effort.

`Result.Metadata.Step` names the rung each name came from. Publish it: traffic
moving down the ladder is the early warning that the short-name space is filling
up, and it arrives well before anyone is handed a suffixed name.

## Filters and failure

Constraints and filters run as a pipeline, cheapest first: repetition (on the
words, before formatting), then length, charset, reserved words, profanity, and
finally your own. Reserved and profanity matching strips case and separators, so
`support` also blocks the slug `sup-port`.

A rejected candidate is retried up to `MaxAttempts` (100 by default). When the
budget runs out you get a `*NoValidCandidateError` with the per-filter
breakdown:

```
names: no valid candidate after 100 attempts (rejected by length=87, reserved=13)
```

Under a seed that failure is **deterministic and permanent** — the same seed
fails identically every time. It is a configuration problem, not a transient
one; retrying will not help. Most such problems are caught at construction
instead: `New` rejects a charset that cannot express the format, and length
bounds no template could satisfy.

`Result.Metadata` carries the attempt count, the selected template, the
contributing packs and the rejection breakdown as plain data — publish it
wherever you already publish metrics; the library depends on no telemetry
system.

## About the shipped vocabulary

The word lists in `packs/` were written for this library rather than sourced from
a corpus, and they have not had a full human review. Curation rules are in
`CONTRIBUTING.md`; mechanical checks (length, alphabet, duplicates) are enforced
by the loader and by CI, and `EstimateCardinality` guards the size in a
regression test. Judgement calls — trademarks, connotations, how a word reads in
another language — are not automatable, and two cleanup passes removed four
trademarks and around twenty invented compounds. Assume some remain.

Treat them as a solid starting point. If your product needs vetted vocabulary,
replace the packs: nothing in the engine changes when you do, and
`Generator.Fingerprint()` tells you exactly when the swap took effect.

## What it does not do

No network. No database. No global state. No LLM. No uniqueness guarantee. It
knows about words, templates, weights, filters, formats and randomness, and
nothing about users, HTTP or any product's business rules.

## Development

```
make test    # go test -race ./...
make lint    # golangci-lint
make deps    # verify no external dependencies
make bench
```

Golden files record the fingerprint alongside the seeded output. When a
vocabulary change is intended:

```
go test -run Golden -update ./...
```

Read the diff before committing: a moved fingerprint means every seeded name in
every consumer's database has stopped being reproducible. That is a changelog
entry, not a rubber stamp.

## License

Apache 2.0.
