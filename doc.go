/*
Package names generates readable, memorable names — usernames, slugs, display
names, workspace and environment labels — from composable themed vocabularies.

	import names "github.com/improbabilitea/names-go"

The module is github.com/improbabilitea/names-go and the package is names; the
-go suffix is a repository convention and does not appear in code.

# Getting started

Register at least one pack, then generate:

	generator, err := names.New(
	    names.WithPack(common.Pack),
	    names.WithFormat(names.Username),
	    names.WithMaxLength(20),
	)
	if err != nil {
	    return err
	}

	result, err := generator.Generate()
	// result.Value == "midnightframe"

Construction is the expensive part and validates everything: a missing category,
a length bound nothing can satisfy, a charset that cannot express the format. A
Generator is immutable afterwards and safe for concurrent use, so build one per
process and share it.

# Packs

A Pack carries dictionaries of words and templates over them. The common pack is
the base; theme packs add their own nouns and lean on the base categories, so
they are registered together and weighted:

	names.WithPack(common.Pack, names.PackWeight(1)),
	names.WithPack(cinema.Pack, names.PackWeight(4)),

Template weights are relative to their own pack and normalized before the pack
weight applies. The pack author controls the proportions inside a theme; the
consumer controls the balance between themes. Adding a template to a pack
redistributes weight within that pack and takes none from the others.

Custom packs load from any fs.FS, so an embed.FS and a directory on disk work
identically:

	pack, err := names.LoadFS(os.DirFS("/etc/myproduct/names"))

# Determinism

A seed makes generation reproducible:

	generator.Generate(names.Seed("user_01893"))

The seed is hashed together with the algorithm version and the pack fingerprint,
so it never reaches the output — passing an email or an internal identifier
leaks nothing — and consecutive seeds produce uncorrelated names.

The guarantee is precise and narrow:

	Seeded output is stable for a given algorithm version and pack fingerprint.
	It is not stable across pack versions.

Any change to a vocabulary moves the fingerprint and re-rolls every seeded name,
because weighted selection depends on the whole word set. There is no middle
ground, and the library does not pretend otherwise: Generator.Fingerprint
exposes the value so a consumer can detect the change instead of discovering it
as silently different names.

The practical consequence: if a name must be stable for years, persist the
generated name, not the seed. Seeded generation is the right tool for fixtures,
tests, ephemeral environments and regenerable identifiers.

# Uniqueness

The library does not guarantee uniqueness and holds no state. Generate a
candidate, insert it, and let a unique constraint arbitrate:

	INSERT -> unique violation -> generate another

Checking availability before writing is a race. GenerateUnique exists for the
cases where a read-then-act is genuinely acceptable, and takes the check from
the caller so the engine stays ignorant of any datastore.

GenerateN returns several names at once, distinct from each other, which is what
to use when offering a choice. Calling Generate repeatedly is not equivalent:
each call derives its stream from the seed alone, so n seeded calls return the
same name n times.

At scale, configure an escalation ladder:

	names.WithEscalation(
	    names.StepTemplateArity(2),   // velvetfox
	    names.StepTemplateArity(3),   // crimsonvelvetfox
	    names.StepNumericSuffix(3),   // smoothcut383
	)

Rungs are tried in order, and both a filter rejection and a candidate the caller
reports as taken consume an attempt — so real collisions drive the descent.

This is not equivalent to weighting three-word templates more heavily. Two draws
collide with probability Σ p²/space over the templates, which is not an average:
a small-space template with non-trivial weight dominates the sum, and large-space
templates alongside it cannot compensate. A ladder exhausts one rung before using
the next, so the spaces add instead of averaging. For the shipped packs that is
the difference between 232 thousand effective names and 262 million.

EstimateCardinality reports the resulting size, and which template is throttling
it.

# Filters and constraints

Constraints (length, charset) and filters run as a pipeline, cheapest first.
Token filters see the words before formatting, where the seams between them are
still visible; string filters see the finished name. A rejected candidate is
retried up to MaxAttempts, after which Generate returns a NoValidCandidateError
carrying the per-filter breakdown.

Reserved words and profanity are matched with case and separators stripped, so
"support" also blocks the slug "sup-port".

# Design boundary

This package knows about words, templates, weights, filters, formats and
randomness. It knows nothing about users, databases, HTTP or any product's
business rules, and it performs no I/O of any kind. It has no dependencies
outside the standard library.
*/
package names
