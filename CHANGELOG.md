# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/). Tags are created by semantic-release
from the commit history; see CONTRIBUTING.md for which prefixes move which
number.

Seeded output is **not** covered by semantic versioning. It is stable only for a
given `algorithmVersion` and `packFingerprint` — see "Determinism" in the README.
Any release that changes a built-in pack changes its fingerprint, and is noted
here under **Changed** with the new fingerprint.

## [1.0.2] — 2026-08-16

### Added

- `names` engine: `New`/`MustNew` with functional options, `Generate`,
  `GenerateN`, `GenerateUnique`, `LoadFS`.
- Five output formats: `Username`, `Slug`, `DisplayName`, `Snake`, `Pascal`.
- Composable packs with per-pack weighting. Template weights are normalized
  within a pack before `PackWeight` applies, so a pack author controls the
  proportions inside a theme and the consumer controls the mix between themes.
- Deterministic seeded generation. The seed is hashed with the algorithm version
  and the pack fingerprint, so it never reaches the output.
- `Generator.Fingerprint`, identifying the merged pack set by content, and
  carried on every `Result`.
- Constraints: minimum and maximum length (in runes, applied after formatting),
  and charset. Contradictory configurations are rejected at construction.
- Filters: built-in repetition and reserved-word filters, plus custom
  `TokenFilter` and `StringFilter` hooks.
- `*NoValidCandidateError` carrying the attempt count and per-filter rejection
  breakdown; `*PackValidationError` carrying pack, field and reason. Both
  classify under sentinels for `errors.Is`.
- Two-level pack validation: `LoadFS` checks a pack in isolation, `New` checks
  the merged set — which is where a theme pack registered without its base fails.
- Escalation ladder (`WithEscalation`, `StepTemplateArity`, `StepNumericSuffix`)
  for name spaces that must stay unique at scale. Rungs are tried in order, and
  both a filter rejection and a candidate the caller reports as taken consume an
  attempt. `Result.Metadata.Step` names the rung a name came from.
- `GenerateN` returns names distinct from each other within the batch; a repeat
  is treated as a filter rejection and escalates like one.
- `Generator.EstimateCardinality`, reporting the effective name-space size, the
  users it supports at a given collision rate, and which template is throttling
  it.
- `packs/common`: 1296 curated words across seven categories, fourteen templates
  including five three-word ones.
- `packs/cinema`: 152 film and theatre nouns, seven templates.
- `filters/reserved`: default list of impersonation and route-collision terms.
- `filters/profanity`: optional, two-stage (exact on words, substring on the
  joined name), kept out of the core.
- Golden, property, fuzz and concurrency tests; benchmarks.

### Notes

- Requires Go 1.26.
- No dependencies outside the standard library, enforced in CI.
- The shipped vocabulary has not had a full human review — see "About the
  shipped vocabulary" in the README. Semantic versioning covers the API; word
  lists change under it, as minor releases.
