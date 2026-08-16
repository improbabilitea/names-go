package names

import (
	"fmt"
	"slices"
	"strings"
)

// Cardinality estimates how many distinct names a configuration can produce.
//
// It exists because "the vocabulary looks big" is not a number, and when names
// have to be unique the number is the requirement. Reach for it before growing a
// word list: it says which template is actually throttling the space, which is
// rarely the one a reader would guess.
type Cardinality struct {
	// Effective is the collision-weighted size of the whole ladder — the figure
	// to compare against an expected user count.
	//
	// It is not the number of distinct names that exist. Weighted selection
	// revisits likely names long before it exhausts unlikely ones, so what
	// matters is the size a collision *behaves* like, which is always smaller
	// than Upper and often by a lot.
	Effective float64

	// Upper is the union of every template's combinations: the count that would
	// apply if selection were uniform and no filter rejected anything. Useful as
	// a ceiling, misleading as a target.
	Upper float64

	// Steps breaks the estimate down by escalation rung, in ladder order.
	Steps []StepCardinality
}

// StepCardinality is the estimate for one rung of the ladder.
type StepCardinality struct {
	Step      string
	Effective float64
	Upper     float64

	// Templates is sorted by collision contribution, worst first — which is the
	// order to read it in, because the top entry is almost always where the
	// cardinality is being lost.
	Templates []TemplateCardinality
}

// TemplateCardinality is one template's contribution.
type TemplateCardinality struct {
	Pattern []string
	Pack    string

	// Probability is this template's share of draws within its rung.
	Probability float64

	// Space is the number of combinations it can produce, suffix included.
	Space float64

	// CollisionShare is the fraction of the rung's collisions this template
	// causes. A template with a small Space and a healthy Probability can
	// dominate this figure while looking innocuous in the manifest — that is the
	// whole reason this field exists.
	CollisionShare float64
}

// EstimateCardinality computes the estimate for this generator.
//
// # The arithmetic
//
// Two independent draws collide with probability Σ p²/space over the templates,
// so the effective size is the reciprocal of that sum. This is not an average:
// a template with a small space and non-trivial weight dominates the sum, and no
// amount of large-space templates alongside it will compensate. That single fact
// is why a wide vocabulary alone does not buy uniqueness, and why the escalation
// ladder exists.
//
// Rungs add rather than average, because the ladder exhausts one before using
// the next. So a ladder's Effective is the sum of its rungs'.
//
// The estimate is an upper bound on reality: filters reject some combinations,
// different templates can coincidentally produce the same string, and the count
// takes no notice of either.
func (g *Generator) EstimateCardinality() Cardinality {
	estimate := Cardinality{Steps: make([]StepCardinality, 0, len(g.escalation))}

	for i := range g.escalation {
		step := g.estimateStep(&g.escalation[i])
		estimate.Steps = append(estimate.Steps, step)
		estimate.Effective += step.Effective
		estimate.Upper += step.Upper
	}

	return estimate
}

func (g *Generator) estimateStep(step *compiledStep) StepCardinality {
	out := StepCardinality{
		Step:      step.label,
		Templates: make([]TemplateCardinality, 0, len(step.templates)),
	}

	total := float64(step.table.Total())
	if total == 0 {
		return out
	}

	suffix := step.suffixSpace()

	var collisions float64
	for i, index := range step.templates {
		template := &g.templates[index]

		space := suffix
		for _, dictionary := range template.dictionaries {
			space *= float64(len(dictionary.words))
		}
		if space == 0 {
			continue
		}

		probability := float64(step.table.WeightAt(i)) / total
		contribution := probability * probability / space
		collisions += contribution

		out.Upper += space
		out.Templates = append(out.Templates, TemplateCardinality{
			Pattern:     slices.Clone(template.pattern),
			Pack:        template.pack,
			Probability: probability,
			Space:       space,
		})
	}

	if collisions > 0 {
		out.Effective = 1 / collisions
		for i := range out.Templates {
			t := &out.Templates[i]
			t.CollisionShare = (t.Probability * t.Probability / t.Space) / collisions
		}
	}

	slices.SortFunc(out.Templates, func(a, b TemplateCardinality) int {
		switch {
		case a.CollisionShare > b.CollisionShare:
			return -1
		case a.CollisionShare < b.CollisionShare:
			return 1
		default:
			return slices.Compare(a.Pattern, b.Pattern)
		}
	})

	return out
}

// UsersAt returns roughly how many names can be assigned before a fresh
// generation collides with probability rate.
//
// With n of m names taken, a fresh draw collides with probability n/m, so this
// is simply rate × Effective. Use it to size a vocabulary against an expected
// user count: at a 1% rate the retry cost is invisible, at 10% it is noticeable
// but survivable, and past 50% the attempt budget starts failing outright.
func (c Cardinality) UsersAt(rate float64) float64 {
	if rate <= 0 || rate > 1 {
		return 0
	}
	return c.Effective * rate
}

// String renders the estimate as a readable report.
func (c Cardinality) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "effective %s   upper %s\n", humanize(c.Effective), humanize(c.Upper))
	fmt.Fprintf(&b, "users at 1%% collisions: %s   at 10%%: %s\n\n",
		humanize(c.UsersAt(0.01)), humanize(c.UsersAt(0.10)))

	for _, step := range c.Steps {
		fmt.Fprintf(&b, "%s — effective %s, upper %s\n",
			step.Step, humanize(step.Effective), humanize(step.Upper))

		for _, t := range step.Templates {
			marker := "  "
			if t.CollisionShare > 0.25 {
				marker = "! " // this one is costing you the space
			}
			fmt.Fprintf(&b, "%s%-40s p=%5.1f%%  space=%12s  collisions=%5.1f%%\n",
				marker, strings.Join(t.Pattern, "+"),
				t.Probability*100, humanize(t.Space), t.CollisionShare*100)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func humanize(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.1fG", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fk", v/1e3)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}
