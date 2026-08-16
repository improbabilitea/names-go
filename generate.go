package names

import (
	"context"
	"fmt"
	"slices"

	"github.com/improbabilitea/names-go/internal/random"
)

// CheckFunc reports whether a candidate is still available.
//
// The library never talks to a datastore itself, so this is how uniqueness gets
// decided: the consumer owns the query, and the engine stays ignorant of
// Postgres, DynamoDB and everything else.
type CheckFunc func(ctx context.Context, candidate string) (available bool, err error)

// Generate produces one name.
//
// Without a Seed the result varies between calls. With one it is reproducible
// for as long as the algorithm version and the pack fingerprint hold — see
// Fingerprint for what that promise does and does not cover.
//
// Generate takes no context: it performs no I/O, cannot block, and has nothing
// to cancel beyond a bounded retry loop.
func (g *Generator) Generate(opts ...GenerateOption) (Result, error) {
	cursor, err := g.cursor(opts)
	if err != nil {
		return Result{}, err
	}

	result, ok := cursor.next()
	if !ok {
		return Result{}, cursor.exhausted()
	}
	return result, nil
}

// GenerateN produces n names, all distinct from each other.
//
// This is the call for offering a user a choice of names, or for seeding
// fixtures. The names come from a single stream rather than n independent ones,
// so a seeded call yields a reproducible *sequence* — whereas calling Generate n
// times with the same seed would return the same name n times, since each call
// derives its stream from the seed alone.
//
// Distinctness is within the batch only. It says nothing about names already in
// a datastore: that is what GenerateUnique and a unique constraint are for. But
// a batch is a set of alternatives, and the same name appearing twice in it is
// never what the caller wanted, so a repeat is treated exactly like a filter
// rejection — it consumes an attempt and, with a ladder configured, eventually
// escalates.
//
// Asking for more names than the vocabulary can distinctly produce therefore
// fails with ErrNoValidCandidate rather than returning duplicates.
func (g *Generator) GenerateN(n int, opts ...GenerateOption) ([]Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("%w: GenerateN needs a positive count, got %d", ErrInvalidOptions, n)
	}

	cursor, err := g.cursor(opts)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, n)
	results := make([]Result, 0, n)

	for range n {
		// Each name gets the full ladder: a batch is n independent generations
		// that happen to share a stream, not one walk down the rungs.
		cursor.rewind()

		for {
			result, ok := cursor.next()
			if !ok {
				return nil, cursor.exhausted()
			}

			if _, duplicate := seen[result.Value]; duplicate {
				cursor.reject("duplicate")
				continue
			}

			seen[result.Value] = struct{}{}
			results = append(results, result)
			break
		}
	}

	return results, nil
}

// GenerateUnique produces a name the consumer's check accepts.
//
// A candidate the check rejects consumes an attempt exactly as a filter
// rejection does, so real collisions are what drive the escalation ladder down
// its rungs. That is the mechanism that keeps early users on short names and
// later ones unique, and it only works if the check reflects the datastore.
//
// It remains the thin version of uniqueness. The robust pattern is to insert and
// let a unique constraint arbitrate, retrying on violation, because checking
// availability and then writing is a race. Use this when a read-then-act is
// genuinely acceptable — reserving a name in a UI preview, say.
//
// A check returning an error stops the loop: an unreachable datastore is not a
// reason to burn the attempt budget.
func (g *Generator) GenerateUnique(ctx context.Context, check CheckFunc, opts ...GenerateOption) (Result, error) {
	if check == nil {
		return Result{}, fmt.Errorf("%w: GenerateUnique needs a check function", ErrInvalidOptions)
	}

	cursor, err := g.cursor(opts)
	if err != nil {
		return Result{}, err
	}

	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}

		result, ok := cursor.next()
		if !ok {
			return Result{}, cursor.exhausted()
		}

		available, err := check(ctx, result.Value)
		if err != nil {
			return Result{}, fmt.Errorf("names: availability check failed: %w", err)
		}
		if available {
			return result, nil
		}

		cursor.reject("taken")
	}
}

// cursor resolves the per-call options and prepares a walk down the ladder.
func (g *Generator) cursor(opts []GenerateOption) (*candidateCursor, error) {
	gc := genConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&gc)
		}
	}

	format := g.format
	if gc.hasFormat {
		if !gc.format.valid() {
			return nil, fmt.Errorf("%w: format %d", ErrUnsupportedFormat, gc.format)
		}
		format = gc.format
	}

	return &candidateCursor{
		g:      g,
		source: g.source(gc),
		format: format,
		words:  make([]string, 0, 4),
		packs:  make([]string, 0, 4),
	}, nil
}

// source returns the randomness for one call.
//
// It is created per call and never stored on the Generator: that is what makes a
// shared Generator safe across goroutines without a mutex, and a PCG is two
// words of state, so the allocation is not worth avoiding.
//
// A Seed always wins over a configured entropy source. The two are contradictory
// requests — reproducible and unpredictable — and a caller who passed a seed
// wants the seed honoured.
func (g *Generator) source(gc genConfig) Source {
	if gc.hasSeed {
		return random.Derive(algorithmVersion, g.fingerprint.bytes(), gc.seed)
	}
	return g.newSource()
}

// candidateCursor walks the escalation ladder, producing candidates that survive
// the filter pipeline.
//
// It exists so that Generate and GenerateUnique share one notion of progress: a
// name rejected by a filter and a name the datastore reports as taken advance
// the same counters and, eventually, the same rung.
type candidateCursor struct {
	g      *Generator
	source Source
	format Format

	step       int
	usedInStep int

	attempts   int
	rejections map[string]int

	words []string
	packs []string
}

// next returns the next candidate that passed every filter, or false once the
// ladder is spent.
func (c *candidateCursor) next() (Result, bool) {
	for c.step < len(c.g.escalation) {
		step := &c.g.escalation[c.step]

		if c.usedInStep >= step.attempts {
			c.step++
			c.usedInStep = 0
			continue
		}
		c.usedInStep++
		c.attempts++

		template := &c.g.templates[step.templates[step.table.Pick(c.source)]]

		c.words, c.packs = c.words[:0], c.packs[:0]
		for _, dictionary := range template.dictionaries {
			word := dictionary.words[dictionary.table.Pick(c.source)]
			c.words = append(c.words, word.value)
			c.packs = appendUnique(c.packs, word.pack)
		}

		// Token filters see the vocabulary words only. A numeric suffix is not a
		// word and has no business being checked for repetition or profanity.
		if rejectedBy := c.g.filters.allowTokens(c.words); rejectedBy != "" {
			c.reject(rejectedBy)
			continue
		}

		tokens := step.appendSuffix(c.words, c.source)
		value := c.format.apply(tokens)

		if rejectedBy := c.g.filters.allowString(value); rejectedBy != "" {
			c.reject(rejectedBy)
			continue
		}

		return Result{
			Value: value,
			Words: slices.Clone(tokens),
			Metadata: Metadata{
				Attempts:    c.attempts,
				Template:    slices.Clone(template.pattern),
				Packs:       slices.Clone(c.packs),
				Step:        step.label,
				Rejections:  c.rejections,
				Fingerprint: c.g.fingerprintText,
			},
		}, true
	}

	return Result{}, false
}

// rewind puts the cursor back at the first rung for the next name in a batch,
// and clears the per-name bookkeeping with it.
//
// Metadata describes the generation of *one* name, so the attempt tally and the
// rejection counts have to reset here. Letting them accumulate would make the
// thousandth name in a batch report a thousand attempts, which is true of the
// call and meaningless as a property of the name.
func (c *candidateCursor) rewind() {
	c.step = 0
	c.usedInStep = 0
	c.attempts = 0
	c.rejections = nil
}

// reject records a rejection against the filter, or the caller, that caused it.
func (c *candidateCursor) reject(reason string) {
	if c.rejections == nil {
		// Allocated only once something is rejected, so the common case — the
		// first candidate accepted — allocates nothing.
		c.rejections = make(map[string]int, 2)
	}
	c.rejections[reason]++
}

// exhausted describes a ladder that ran out of rungs.
//
// Under a seed this failure is deterministic and permanent: the same seed against
// the same configuration fails identically every time. It is a configuration
// problem, not a transient one, and retrying at the call site will not help.
func (c *candidateCursor) exhausted() error {
	return &NoValidCandidateError{
		Attempts:   c.attempts,
		Rejections: c.rejections,
	}
}

// uint64N reduces a random value into [0, n), using the library's own reduction
// rather than the standard library's — see internal/random for why.
func uint64N(source Source, n uint64) uint64 {
	return random.Uint64N(source, n)
}

func appendUnique(dst []string, s string) []string {
	if slices.Contains(dst, s) {
		return dst
	}
	return append(dst, s)
}
