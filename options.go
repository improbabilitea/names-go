package names

import (
	"fmt"

	"github.com/improbabilitea/names-go/internal/random"
)

// Source produces uniformly distributed 64-bit values.
//
// Only Uint64 is required. Reducing a value into a range is done inside the
// library rather than by the source, so that seeded output cannot shift when a
// standard library reduction algorithm changes underneath it.
type Source interface {
	Uint64() uint64
}

// Entropy factories for WithEntropy. Each call must return an independent
// source, because generation takes one per call — that is what keeps a shared
// Generator lock-free.
var (
	// PCGEntropy is the default: fast, and seeded from the goroutine-safe
	// global source.
	PCGEntropy = func() Source { return random.NewPCG() }

	// CryptoEntropy draws from crypto/rand. Opt into it when the generated name
	// is a public identifier that must not be predictable — PCG is reproducible
	// by design and can be reconstructed from a little output.
	CryptoEntropy = func() Source { return random.NewCrypto() }
)

// Option configures a Generator at construction.
type Option func(*config) error

// PackOption configures how a single pack participates in the mix.
type PackOption func(*packConfig)

type packConfig struct {
	weight uint32
}

// config is the accumulated result of the options, before compilation.
type config struct {
	packs []registeredPack

	format    Format
	minLength int
	maxLength int
	charset   Charset

	reserved      []string
	tokenFilters  []TokenFilter
	stringFilters []StringFilter
	repetition    bool

	escalation  []EscalationStep
	maxAttempts int
	newSource   func() Source
}

func defaultConfig() config {
	return config{
		format:      Username,
		repetition:  true,
		maxAttempts: 100,
		newSource:   PCGEntropy,
	}
}

// WithPack registers a pack, optionally with the weight it carries in the mix.
//
// Registration order is significant: it decides which pack wins a duplicated
// word, and it is part of the determinism contract, so reordering these calls
// changes the fingerprint and re-rolls seeded output.
//
//	names.WithPack(common.Pack, names.PackWeight(1)),
//	names.WithPack(cinema.Pack, names.PackWeight(4)),
func WithPack(p *Pack, opts ...PackOption) Option {
	return func(c *config) error {
		if p == nil {
			return fmt.Errorf("%w: WithPack was given a nil pack", ErrInvalidOptions)
		}

		pc := packConfig{weight: defaultWeight}
		for _, opt := range opts {
			opt(&pc)
		}
		if pc.weight == 0 {
			return fmt.Errorf("%w: pack %q was given weight 0; omit PackWeight for the default",
				ErrInvalidOptions, p.Name())
		}

		c.packs = append(c.packs, registeredPack{pack: p, weight: pc.weight})
		return nil
	}
}

// PackWeight sets how much a pack counts in the mix, relative to the others.
//
// Template weights are relative to their own pack and are normalized before this
// is applied, so a pack author controls the proportions inside their theme while
// the consumer controls the balance between themes. Registering common at 1 and
// cinema at 4 means four names in five come from a cinema template, whatever
// weights either pack declares internally.
func PackWeight(w uint32) PackOption {
	return func(pc *packConfig) { pc.weight = w }
}

// WithFormat sets the output format. Defaults to Username.
func WithFormat(f Format) Option {
	return func(c *config) error {
		if !f.valid() {
			return fmt.Errorf("%w: format %d", ErrUnsupportedFormat, f)
		}
		c.format = f
		return nil
	}
}

// WithMinLength rejects names shorter than n runes. Zero means no minimum.
func WithMinLength(n int) Option {
	return func(c *config) error {
		if n < 0 {
			return fmt.Errorf("%w: MinLength must not be negative, got %d", ErrInvalidOptions, n)
		}
		c.minLength = n
		return nil
	}
}

// WithMaxLength rejects names longer than n runes. Zero means no maximum.
//
// The limit applies to the *formatted* name, so it counts the separators a slug
// or a display name introduces: the same candidate can fit as a username and be
// too long as a slug.
func WithMaxLength(n int) Option {
	return func(c *config) error {
		if n < 0 {
			return fmt.Errorf("%w: MaxLength must not be negative, got %d", ErrInvalidOptions, n)
		}
		c.maxLength = n
		return nil
	}
}

// WithCharset restricts the alphabet of the formatted name.
//
// The default is unrestricted, which suits a display name. A charset that cannot
// express the configured format — lowercase-only against Slug, which needs a
// hyphen — is rejected by New rather than left to fail every candidate at
// runtime.
func WithCharset(c Charset) Option {
	return func(cfg *config) error {
		cfg.charset = c
		return nil
	}
}

// WithReservedWords blocks terms the product keeps for itself.
//
// Matching ignores case and separators, so "support" blocks the slug
// "sup-port" too. Repeated calls accumulate.
func WithReservedWords(words []string) Option {
	return func(c *config) error {
		c.reserved = append(c.reserved, words...)
		return nil
	}
}

// WithFilter adds a custom filter over the formatted name.
//
// Custom filters run last, after the built-in ones, so they only see candidates
// that already satisfy the length, charset and reserved-word rules.
func WithFilter(f StringFilter) Option {
	return func(c *config) error {
		if f == nil {
			return fmt.Errorf("%w: WithFilter was given a nil filter", ErrInvalidOptions)
		}
		if f.Name() == "" {
			return fmt.Errorf("%w: a filter must have a name, it is what rejections are reported under",
				ErrInvalidOptions)
		}
		c.stringFilters = append(c.stringFilters, f)
		return nil
	}
}

// WithTokenFilter adds a custom filter over the selected words, before
// formatting — the stage where the boundaries between words are still visible.
func WithTokenFilter(f TokenFilter) Option {
	return func(c *config) error {
		if f == nil {
			return fmt.Errorf("%w: WithTokenFilter was given a nil filter", ErrInvalidOptions)
		}
		if f.Name() == "" {
			return fmt.Errorf("%w: a filter must have a name, it is what rejections are reported under",
				ErrInvalidOptions)
		}
		c.tokenFilters = append(c.tokenFilters, f)
		return nil
	}
}

// WithRepetitionCheck enables or disables the built-in repetition filter, which
// is on by default. Turning it off admits names like "silentsilent".
func WithRepetitionCheck(enabled bool) Option {
	return func(c *config) error {
		c.repetition = enabled
		return nil
	}
}

// WithMaxAttempts caps how many candidates a single Generate call may try before
// giving up. Defaults to 100.
func WithMaxAttempts(n int) Option {
	return func(c *config) error {
		if n <= 0 {
			return fmt.Errorf("%w: MaxAttempts must be positive, got %d", ErrInvalidOptions, n)
		}
		c.maxAttempts = n
		return nil
	}
}

// WithEntropy replaces the randomness source used for unseeded generation.
//
// It does not affect seeded generation, which always uses the deterministic
// derivation — a Seed and an unpredictable source are contradictory requests,
// and silently honouring the seed is what a caller who passed one wants.
func WithEntropy(newSource func() Source) Option {
	return func(c *config) error {
		if newSource == nil {
			return fmt.Errorf("%w: WithEntropy was given a nil factory", ErrInvalidOptions)
		}
		c.newSource = newSource
		return nil
	}
}

// GenerateOption adjusts a single Generate call.
//
// Only what genuinely varies per call lives here. Vocabulary, constraints and
// filters belong to the constructor, where they are compiled once — accepting
// them per call would make the hot path rebuild its configuration on every
// request.
type GenerateOption func(*genConfig)

type genConfig struct {
	seed    string
	hasSeed bool

	format    Format
	hasFormat bool
}

// Seed makes this call deterministic.
//
// The same seed, against the same algorithm version and pack fingerprint, always
// produces the same name. The seed is hashed before use and never appears in the
// output, so passing an email or an internal identifier leaks nothing.
func Seed(s string) GenerateOption {
	return func(g *genConfig) {
		g.seed = s
		g.hasSeed = true
	}
}

// UseFormat overrides the configured format for this call.
func UseFormat(f Format) GenerateOption {
	return func(g *genConfig) {
		g.format = f
		g.hasFormat = true
	}
}
