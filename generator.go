package names

import (
	"fmt"
	"slices"

	"github.com/improbabilitea/names-go/internal/weighted"
)

// compiledWord is a selectable word plus the pack it came from, which is what
// lets a Result report which packs contributed to it.
//
// The originating pack is tracked here rather than on Word so the public data
// model stays about vocabulary, not about bookkeeping.
type compiledWord struct {
	value string
	pack  string
}

// compiledDictionary is a merged category, ready to select from.
type compiledDictionary struct {
	words []compiledWord
	table weighted.Table

	// shortest and longest are rune lengths, kept to detect at construction
	// time that a length constraint has made every candidate impossible.
	shortest int
	longest  int
}

type compiledTemplate struct {
	pattern []string
	pack    string

	// dictionaries mirrors pattern, resolved once at construction. Generation
	// walks this instead of looking each category up by name: the hot path runs
	// per word per attempt, and a map lookup there buys nothing that
	// construction cannot pay for once.
	dictionaries []*compiledDictionary
}

// Generator produces names. It is immutable once built.
//
// Nothing is mutated during generation — the vocabulary, the weight tables and
// the filter pipeline are all read-only, and the randomness source is created
// per call — so a Generator is safe to share across goroutines without a lock.
// Build one per process and reuse it; construction is the expensive part and it
// happens exactly once.
type Generator struct {
	fingerprint fingerprint

	// fingerprintText is the rendered form, computed once. It goes into every
	// Result, and hex-encoding a constant on each generation is pure waste on
	// the hot path.
	fingerprintText string

	packNames []string
	warnings  []string

	templates     []compiledTemplate
	templateTable weighted.Table

	// escalation is the ladder, always non-empty: without WithEscalation it
	// holds a single rung over every template, which is the old behaviour.
	escalation []compiledStep

	format      Format
	filters     pipeline
	maxAttempts int
	newSource   func() Source
}

// New builds a Generator from the given options.
//
// It returns an error rather than panicking because everything that can be wrong
// with a configuration is discovered here — a missing category, a length bound
// nothing can satisfy, a charset that cannot express the format. Surfacing all
// of it at construction is the whole point: these are the failures that would
// otherwise appear on a user's first request.
func New(opts ...Option) (*Generator, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	if err := validateOptions(&cfg); err != nil {
		return nil, err
	}

	dictionaries, warnings := mergeDictionaries(cfg.packs)

	templates, table, err := compileTemplates(cfg.packs)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, ErrNoTemplate
	}

	if err := resolveReferences(templates, dictionaries); err != nil {
		return nil, err
	}
	if err := validateFeasibility(&cfg, templates); err != nil {
		return nil, err
	}

	escalation, ladderWarnings, err := compileEscalation(&cfg, templates, table)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, ladderWarnings...)
	warnings = append(warnings, checkSuffixFits(&cfg, templates, escalation)...)

	packNames := make([]string, 0, len(cfg.packs))
	for _, registered := range cfg.packs {
		packNames = append(packNames, registered.pack.Name())
	}

	print := computeFingerprint(cfg.packs)

	return &Generator{
		fingerprint:     print,
		fingerprintText: print.String(),

		packNames:     packNames,
		warnings:      warnings,
		templates:     templates,
		templateTable: table,
		escalation:    escalation,
		format:        cfg.format,
		filters:       buildPipeline(&cfg),
		maxAttempts:   cfg.maxAttempts,
		newSource:     cfg.newSource,
	}, nil
}

// MustNew is New for package-level variables, where a bad pack should stop the
// process from starting rather than surface on the first request.
func MustNew(opts ...Option) *Generator {
	g, err := New(opts...)
	if err != nil {
		panic("names: " + err.Error())
	}
	return g
}

// Fingerprint identifies the merged pack set by content.
//
// Seeded output is stable for a given algorithm version and fingerprint, and for
// nothing wider than that. Record it alongside anything that depends on seeded
// names — a golden file, a cache — so a vocabulary change is caught rather than
// silently producing different names.
func (g *Generator) Fingerprint() string { return g.fingerprintText }

// Packs returns the registered pack names, in registration order.
func (g *Generator) Packs() []string { return slices.Clone(g.packNames) }

// Warnings returns non-fatal observations from construction, chiefly words
// defined by more than one pack, where the later registration won.
//
// Empty for a healthy configuration. Worth logging once at startup: a silently
// overridden word is the kind of thing that makes a vocabulary behave unlike
// its source files.
func (g *Generator) Warnings() []string { return slices.Clone(g.warnings) }

// --- construction-time validation -------------------------------------------

func validateOptions(cfg *config) error {
	if len(cfg.packs) == 0 {
		return ErrNoPack
	}

	if cfg.minLength > 0 && cfg.maxLength > 0 && cfg.minLength > cfg.maxLength {
		return fmt.Errorf("%w: MinLength %d exceeds MaxLength %d",
			ErrInvalidOptions, cfg.minLength, cfg.maxLength)
	}

	// A charset that cannot spell the format rejects every candidate. Probing it
	// with a representative rendering catches both the obvious case (a slug's
	// hyphen missing from a lowercase-alphanumeric set) and the easily missed
	// one (a lowercase set against a format that capitalizes).
	if missing := cfg.charset.missing(cfg.format.sampleOutput()); missing != "" {
		return fmt.Errorf("%w: charset %q cannot express the %s format, which needs %q",
			ErrInvalidOptions, cfg.charset, cfg.format, missing)
	}

	return nil
}

// resolveReferences binds each template's categories to the merged dictionaries,
// and is the check a theme pack loaded without its base fails.
//
// LoadFS cannot make that check: a cinema pack referencing `adjective` is
// incomplete on its own by design. Only here, with the full set registered, is a
// missing category unambiguously an error — and it is the most likely
// integration mistake there is.
func resolveReferences(templates []compiledTemplate, dictionaries map[string]*compiledDictionary) error {
	for i := range templates {
		t := &templates[i]
		t.dictionaries = make([]*compiledDictionary, 0, len(t.pattern))

		for _, category := range t.pattern {
			dictionary, ok := dictionaries[category]
			if !ok {
				return fmt.Errorf(
					"%w: template %v from pack %q needs category %q, which no registered pack provides",
					ErrDictionaryNotFound, t.pattern, t.pack, category)
			}
			t.dictionaries = append(t.dictionaries, dictionary)
		}
	}
	return nil
}

// validateFeasibility rejects length bounds that no template could ever satisfy.
//
// Without this the misconfiguration surfaces as ErrNoValidCandidate on a user's
// first request, having burned the full attempt budget — a runtime failure
// standing in for what is plainly a configuration mistake.
func validateFeasibility(cfg *config, templates []compiledTemplate) error {
	if cfg.minLength == 0 && cfg.maxLength == 0 {
		return nil
	}

	separator := len([]rune(cfg.format.separator()))

	for _, t := range templates {
		joins := separator * (len(t.pattern) - 1)
		shortest, longest := joins, joins

		for _, d := range t.dictionaries {
			shortest += d.shortest
			longest += d.longest
		}

		tooLong := cfg.maxLength > 0 && shortest > cfg.maxLength
		tooShort := cfg.minLength > 0 && longest < cfg.minLength
		if !tooLong && !tooShort {
			return nil
		}
	}

	return fmt.Errorf(
		"%w: no template can produce a name of %s in the %s format; widen the bounds or the vocabulary",
		ErrInvalidOptions, describeBounds(cfg.minLength, cfg.maxLength), cfg.format)
}

// checkSuffixFits warns about a suffix rung that the length bound makes
// unreachable.
//
// A warning rather than an error: the rungs above it may well be enough, and
// refusing to start would be the wrong response to a ladder whose last resort is
// merely unusable. But a product that reaches a dead rung would get
// ErrNoValidCandidate with no explanation, so it is worth saying at startup.
func checkSuffixFits(cfg *config, templates []compiledTemplate, steps []compiledStep) []string {
	if cfg.maxLength == 0 {
		return nil
	}

	separator := len([]rune(cfg.format.separator()))

	shortestName := -1
	for _, t := range templates {
		length := separator * (len(t.pattern) - 1)
		for _, d := range t.dictionaries {
			length += d.shortest
		}
		if shortestName < 0 || length < shortestName {
			shortestName = length
		}
	}

	var warnings []string
	for _, step := range steps {
		if step.digits == 0 {
			continue
		}
		if shortestName+separator+step.digits > cfg.maxLength {
			warnings = append(warnings, fmt.Sprintf(
				"escalation step %q cannot fit MaxLength %d: the shortest name is %d characters and the suffix adds %d",
				step.label, cfg.maxLength, shortestName, separator+step.digits))
		}
	}
	return warnings
}

func describeBounds(minLength, maxLength int) string {
	switch {
	case minLength > 0 && maxLength > 0:
		return fmt.Sprintf("between %d and %d characters", minLength, maxLength)
	case maxLength > 0:
		return fmt.Sprintf("at most %d characters", maxLength)
	default:
		return fmt.Sprintf("at least %d characters", minLength)
	}
}

// --- merging ----------------------------------------------------------------

// mergedEntry is a word in flight during the merge, before the tables are built.
type mergedEntry struct {
	word   compiledWord
	weight uint32
}

// mergeDictionaries combines the registered packs into one vocabulary.
//
// Categories of the same name merge their contents. A word defined by more than
// one pack resolves to the last registration, which is what makes registration
// order meaningful: a theme pack registered after the base can deliberately
// restate a word with a different weight.
func mergeDictionaries(packs []registeredPack) (map[string]*compiledDictionary, []string) {
	byCategory := make(map[string]map[string]mergedEntry)
	var warnings []string

	for _, registered := range packs {
		for _, dictionary := range registered.pack.dictionaries {
			entries, ok := byCategory[dictionary.Category]
			if !ok {
				entries = make(map[string]mergedEntry)
				byCategory[dictionary.Category] = entries
			}

			for _, word := range dictionary.Words {
				if !word.Enabled {
					continue
				}

				if previous, duplicate := entries[word.Value]; duplicate {
					warnings = append(warnings, fmt.Sprintf(
						"word %q in category %q: pack %q overrides pack %q",
						word.Value, dictionary.Category, registered.pack.Name(), previous.word.pack))
				}

				entries[word.Value] = mergedEntry{
					word:   compiledWord{value: word.Value, pack: registered.pack.Name()},
					weight: word.weight(),
				}
			}
		}
	}

	compiled := make(map[string]*compiledDictionary, len(byCategory))
	for category, entries := range byCategory {
		// Sorted, never a map range: the order of this slice decides which word
		// each random draw lands on, so it has to be a function of the content
		// alone.
		values := sortedKeys(entries)
		if len(values) == 0 {
			continue
		}

		words := make([]compiledWord, 0, len(values))
		weights := make([]uint64, 0, len(values))
		shortest, longest := 0, 0

		for _, value := range values {
			entry := entries[value]
			words = append(words, entry.word)
			weights = append(weights, uint64(entry.weight))

			length := len([]rune(value))
			if shortest == 0 || length < shortest {
				shortest = length
			}
			if length > longest {
				longest = length
			}
		}

		compiled[category] = &compiledDictionary{
			words:    words,
			table:    weighted.New(weights),
			shortest: shortest,
			longest:  longest,
		}
	}

	return compiled, warnings
}

// --- template weight composition --------------------------------------------

// compileTemplates flattens every pack's templates into one weighted table.
//
// # The arithmetic, and why it is exact
//
// A template's declared weight is relative to its own pack. What we need is
//
//	P(template) = P(pack) x P(template | pack)
//
// which means normalizing each pack's templates to sum to the same value before
// scaling by the pack weight. Doing that in floating point would be simpler and
// would put a rounding error on the deterministic path; instead every pack's
// total is brought to their least common multiple, so each template's share
// becomes an exact integer:
//
//	weight = packWeight x templateWeight x (lcm / packTotal)
//
// Each pack then contributes exactly packWeight x lcm to the grand total, which
// is the property that makes PackWeight mean what it says — and makes adding a
// template to a pack redistribute weight *within* that pack rather than stealing
// it from the others.
func compileTemplates(packs []registeredPack) ([]compiledTemplate, weighted.Table, error) {
	totals := make([]uint64, len(packs))
	for i, registered := range packs {
		for _, template := range registered.pack.templates {
			totals[i] += uint64(template.weight())
		}
	}

	common := uint64(1)
	for i, total := range totals {
		if total == 0 {
			continue
		}
		next, ok := lcm(common, total)
		if !ok {
			return nil, weighted.Table{}, unrepresentableWeights(packs[i].pack.Name())
		}
		common = next
	}

	var (
		templates []compiledTemplate
		weights   []uint64
		grand     uint64
	)

	for i, registered := range packs {
		if totals[i] == 0 {
			continue
		}
		factor := common / totals[i]

		for _, template := range registered.pack.templates {
			weight, ok := mulChecked(uint64(registered.weight), uint64(template.weight()))
			if ok {
				weight, ok = mulChecked(weight, factor)
			}
			if !ok {
				return nil, weighted.Table{}, unrepresentableWeights(registered.pack.Name())
			}

			grand, ok = addChecked(grand, weight)
			if !ok {
				return nil, weighted.Table{}, unrepresentableWeights(registered.pack.Name())
			}

			templates = append(templates, compiledTemplate{
				pattern: template.Pattern,
				pack:    registered.pack.Name(),
			})
			weights = append(weights, weight)
		}
	}

	return templates, weighted.New(weights), nil
}

func unrepresentableWeights(pack string) error {
	return fmt.Errorf(
		"%w: the composed template weights overflow 64 bits around pack %q; reduce the declared weights or the pack weights",
		ErrInvalidOptions, pack)
}

func gcd(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func lcm(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	return mulChecked(a/gcd(a, b), b)
}

func mulChecked(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product := a * b
	if product/a != b {
		return 0, false
	}
	return product, true
}

func addChecked(a, b uint64) (uint64, bool) {
	sum := a + b
	return sum, sum >= a
}

// --- filter pipeline --------------------------------------------------------

// buildPipeline assembles the filters in running order.
//
// Cheapest and most-rejecting first, so the costly checks see the fewest
// candidates: repetition works on the tokens and must precede formatting, then
// length and charset are near-free rejections, then the reserved lookup, and
// finally whatever the consumer added.
func buildPipeline(cfg *config) pipeline {
	var p pipeline

	if cfg.repetition {
		p.token = append(p.token, repetitionFilter{})
	}

	var reserved reservedFilter
	hasReserved := len(cfg.reserved) > 0
	if hasReserved {
		reserved = newReservedFilter(cfg.reserved)
		p.token = append(p.token, reserved)
	}

	p.token = append(p.token, cfg.tokenFilters...)

	if cfg.minLength > 0 || cfg.maxLength > 0 {
		p.string = append(p.string, lengthFilter{min: cfg.minLength, max: cfg.maxLength})
	}
	if cfg.charset.isSet() {
		p.string = append(p.string, charsetFilter{charset: cfg.charset})
	}
	if hasReserved {
		p.string = append(p.string, reserved)
	}

	p.string = append(p.string, cfg.stringFilters...)

	return p
}
