package names

import (
	"fmt"
	"strconv"

	"github.com/improbabilitea/names-go/internal/weighted"
)

// EscalationStep is one rung of the collision ladder.
//
// # Why a ladder rather than heavier templates
//
// It is tempting to widen the name space by weighting three-word templates more
// heavily. That does not work, and the reason is worth stating because it is
// counter-intuitive.
//
// The chance two independent draws collide is the sum over templates of
// p²/space. It is not an average: a template with a small space and meaningful
// weight dominates the whole sum, and no amount of large-space templates
// alongside it can compensate. Mixing a 90-thousand space at 30% with a
// 27-million space at 70% yields under a million effective names, because the
// small one contributes nearly all the collisions.
//
// A ladder is not a mixture. It consumes the short-name space deliberately, and
// only moves on once that space is spent — so the two spaces add rather than
// average, and early users get short names while later ones stay unique.
type EscalationStep struct {
	kind     stepKind
	arity    int
	digits   int
	attempts int
}

type stepKind uint8

const (
	stepArity stepKind = iota
	stepSuffix
)

// StepOption configures a single rung.
type StepOption func(*EscalationStep)

// Attempts sets how many candidates this rung may try before the ladder moves
// on. Without it, the rungs share the generator's MaxAttempts equally.
func Attempts(n int) StepOption {
	return func(s *EscalationStep) { s.attempts = n }
}

// StepTemplateArity restricts a rung to templates producing exactly n words.
//
//	names.WithEscalation(
//	    names.StepTemplateArity(2),   // velvetfox
//	    names.StepTemplateArity(3),   // crimsonvelvetfox
//	)
//
// A rung whose arity no registered template provides is skipped, and the fact is
// reported through Warnings at construction rather than discovered at runtime.
func StepTemplateArity(n int, opts ...StepOption) EscalationStep {
	step := EscalationStep{kind: stepArity, arity: n}
	for _, opt := range opts {
		opt(&step)
	}
	return step
}

// StepNumericSuffix appends n random digits to the name.
//
//	names.StepNumericSuffix(2)   // crimsonvelvetfox47
//
// This is the rung of last resort and is opt-in for a reason: it produces
// visibly worse names, and a product that reaches it on the first user has a
// vocabulary problem rather than a cardinality one. Past a certain scale there
// is no alternative — no readable vocabulary reaches a billion distinct names on
// its own — but it belongs at the bottom of the ladder, not in the mix.
//
// The digits are appended as an extra word, so they pick up the format's
// separator: "velvet-fox-47" as a slug, "Velvet Fox 47" as a display name. They
// are zero-padded, which keeps the added space exactly 10^n.
func StepNumericSuffix(n int, opts ...StepOption) EscalationStep {
	step := EscalationStep{kind: stepSuffix, digits: n}
	for _, opt := range opts {
		opt(&step)
	}
	return step
}

// WithEscalation sets the collision ladder.
//
// Rungs are tried in order; the ladder moves on when a rung exhausts its
// attempt budget. Both a filter rejection and, under GenerateUnique, a candidate
// the caller reports as taken consume an attempt — which is what makes the
// ladder respond to real collisions rather than to a guess about them.
//
// Without this option the generator behaves as a single rung over all templates,
// which is the right default when names need not be unique.
func WithEscalation(steps ...EscalationStep) Option {
	return func(c *config) error {
		if len(steps) == 0 {
			return fmt.Errorf("%w: WithEscalation needs at least one step", ErrInvalidOptions)
		}
		for i, s := range steps {
			if s.attempts < 0 {
				return fmt.Errorf("%w: escalation step %d has a negative attempt budget", ErrInvalidOptions, i)
			}
			if s.kind == stepArity && s.arity < 1 {
				return fmt.Errorf("%w: escalation step %d has arity %d, want at least 1",
					ErrInvalidOptions, i, s.arity)
			}
			if s.kind == stepSuffix && (s.digits < 1 || s.digits > 18) {
				return fmt.Errorf("%w: escalation step %d has %d digits, want 1 to 18",
					ErrInvalidOptions, i, s.digits)
			}
		}
		c.escalation = steps
		return nil
	}
}

// compiledStep is a rung with its template table resolved.
type compiledStep struct {
	templates []int // indices into Generator.templates
	table     weighted.Table
	digits    int
	attempts  int
	label     string
}

// suffixSpace is how many distinct suffixes this rung adds, or 1 if none.
func (s compiledStep) suffixSpace() float64 {
	if s.digits == 0 {
		return 1
	}
	space := 1.0
	for range s.digits {
		space *= 10
	}
	return space
}

// draw appends the rung's suffix, if it has one, to the selected words.
func (s compiledStep) appendSuffix(words []string, source Source) []string {
	if s.digits == 0 {
		return words
	}
	bound := uint64(1)
	for range s.digits {
		bound *= 10
	}
	value := uint64N(source, bound)
	return append(words, padDigits(value, s.digits))
}

func padDigits(value uint64, digits int) string {
	text := strconv.FormatUint(value, 10)
	if len(text) >= digits {
		return text
	}
	padded := make([]byte, digits)
	pad := digits - len(text)
	for i := range pad {
		padded[i] = '0'
	}
	copy(padded[pad:], text)
	return string(padded)
}

// compileEscalation resolves the configured ladder against the available
// templates.
//
// Returns the rungs, plus warnings for any that no template can serve — a rung
// asking for three-word names in a pack set that has none is a configuration
// mistake worth reporting, but not worth refusing to start over.
func compileEscalation(cfg *config, templates []compiledTemplate, full weighted.Table) ([]compiledStep, []string, error) {
	if len(cfg.escalation) == 0 {
		return []compiledStep{{
			templates: allIndices(len(templates)),
			table:     full,
			attempts:  cfg.maxAttempts,
			label:     "all templates",
		}}, nil, nil
	}

	budgets, err := shareAttempts(cfg)
	if err != nil {
		return nil, nil, err
	}

	var (
		compiled []compiledStep
		warnings []string
	)

	for i, step := range cfg.escalation {
		switch step.kind {
		case stepArity:
			indices, table := templatesOfArity(templates, full, step.arity)
			if len(indices) == 0 {
				warnings = append(warnings, fmt.Sprintf(
					"escalation step %d asks for %d-word names, which no registered template produces; the step is skipped",
					i, step.arity))
				continue
			}
			compiled = append(compiled, compiledStep{
				templates: indices,
				table:     table,
				attempts:  budgets[i],
				label:     fmt.Sprintf("%d-word templates", step.arity),
			})

		case stepSuffix:
			compiled = append(compiled, compiledStep{
				templates: allIndices(len(templates)),
				table:     full,
				digits:    step.digits,
				attempts:  budgets[i],
				label:     fmt.Sprintf("%d-digit suffix", step.digits),
			})
		}
	}

	if len(compiled) == 0 {
		return nil, nil, fmt.Errorf(
			"%w: every escalation step was skipped, so no name can be produced", ErrNoTemplate)
	}

	return compiled, warnings, nil
}

// shareAttempts assigns each rung its budget.
//
// MaxAttempts is the total across the ladder. A rung that names its own budget
// takes it; the rest split what remains. Declaring more than the total is a
// configuration error rather than a silent overrun, because the caller has asked
// for two contradictory things.
func shareAttempts(cfg *config) ([]int, error) {
	budgets := make([]int, len(cfg.escalation))

	claimed, unspecified := 0, 0
	for i, s := range cfg.escalation {
		if s.attempts > 0 {
			budgets[i] = s.attempts
			claimed += s.attempts
			continue
		}
		unspecified++
	}

	if claimed > cfg.maxAttempts {
		return nil, fmt.Errorf(
			"%w: escalation steps claim %d attempts but MaxAttempts is %d",
			ErrInvalidOptions, claimed, cfg.maxAttempts)
	}

	if unspecified > 0 {
		remaining := cfg.maxAttempts - claimed
		each := max(remaining/unspecified, 1)
		for i, s := range cfg.escalation {
			if s.attempts == 0 {
				budgets[i] = each
			}
		}
	}

	return budgets, nil
}

func templatesOfArity(templates []compiledTemplate, full weighted.Table, arity int) ([]int, weighted.Table) {
	var (
		indices []int
		weights []uint64
	)

	previous := uint64(0)
	for i, t := range templates {
		cumulative := full.CumulativeAt(i)
		weight := cumulative - previous
		previous = cumulative

		if len(t.pattern) == arity {
			indices = append(indices, i)
			weights = append(weights, weight)
		}
	}

	return indices, weighted.New(weights)
}

func allIndices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
