package names

import (
	"math"
	"strings"
	"testing"
)

func closeTo(t *testing.T, label string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}

// TestEstimateCardinality_SingleTemplate pins the arithmetic against a case
// small enough to count by hand: two adjectives and three animals is six names.
func TestEstimateCardinality_SingleTemplate(t *testing.T) {
	pack := singleWordPack(t, "p", []string{"a", "b"}, map[string][]string{
		"a": {"velvet", "silent"},
		"b": {"fox", "owl", "hare"},
	})

	estimate := mustNew(t, WithPack(pack)).EstimateCardinality()

	closeTo(t, "Effective", estimate.Effective, 6, 0.001)
	closeTo(t, "Upper", estimate.Upper, 6, 0.001)

	if len(estimate.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(estimate.Steps))
	}
	if len(estimate.Steps[0].Templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(estimate.Steps[0].Templates))
	}
	closeTo(t, "Probability", estimate.Steps[0].Templates[0].Probability, 1, 0.001)
}

// TestEstimateCardinality_TheSmallSpaceDominates is the finding the whole
// escalation ladder rests on, so it gets a test with numbers a reader can check.
//
// Two templates at equal weight, one with 4 combinations and one with 100. The
// naive expectation is something near the middle. What actually comes out is 15,
// because the small template causes 96% of the collisions — and that is why
// weighting a large template more heavily cannot rescue a mix containing a small
// one.
func TestEstimateCardinality_TheSmallSpaceDominates(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{"name":"p","dictionaries":{"tiny":"tiny.txt","big":"big.txt"},
		  "templates":[
		    {"pattern":["tiny","tiny"],"weight":1},
		    {"pattern":["big","big"],"weight":1}]}`,
		"tiny.txt": "aa\nbb\n",
		"big.txt":  buildWords(10),
	})

	estimate := mustNew(t, WithPack(pack), WithRepetitionCheck(false)).EstimateCardinality()

	// sum = 0.5²/4 + 0.5²/100 = 0.0625 + 0.0025 = 0.065  ->  1/0.065 = 15.38
	closeTo(t, "Effective", estimate.Effective, 15.384, 0.01)
	closeTo(t, "Upper", estimate.Upper, 104, 0.001)

	worst := estimate.Steps[0].Templates[0]
	if worst.Space != 4 {
		t.Fatalf("the worst template has space %.0f, want the small one at 4", worst.Space)
	}
	closeTo(t, "CollisionShare", worst.CollisionShare, 0.9615, 0.001)

	// The headline: effective size is far below even the smaller half of the mix
	// weighted by hand, and nowhere near the arithmetic mean of 52.
	if estimate.Effective > 20 {
		t.Errorf("Effective = %.1f; the small template is not dominating as it should", estimate.Effective)
	}
}

// TestEstimateCardinality_RungsAdd is the other half: a ladder consumes one rung
// before the next, so the sizes add instead of averaging. That is the difference
// between 15 names and 104.
func TestEstimateCardinality_RungsAdd(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{"name":"p","dictionaries":{"a":"a.txt","b":"b.txt"},
		  "templates":[
		    {"pattern":["a","a"],"weight":1},
		    {"pattern":["b","b","b"],"weight":1}]}`,
		"a.txt": "aa\nbb\n",
		"b.txt": buildWords(10),
	})

	g := mustNew(t,
		WithPack(pack),
		WithRepetitionCheck(false),
		WithEscalation(StepTemplateArity(2), StepTemplateArity(3)),
	)
	estimate := g.EstimateCardinality()

	if len(estimate.Steps) != 2 {
		t.Fatalf("got %d rungs, want 2", len(estimate.Steps))
	}
	closeTo(t, "rung 1", estimate.Steps[0].Effective, 4, 0.001)
	closeTo(t, "rung 2", estimate.Steps[1].Effective, 1000, 0.001)
	closeTo(t, "total", estimate.Effective, 1004, 0.001)
}

// TestEstimateCardinality_SuffixMultiplies checks the digits are counted, since
// they are the reason a suffix rung exists at all.
func TestEstimateCardinality_SuffixMultiplies(t *testing.T) {
	pack := singleWordPack(t, "p", []string{"a", "b"}, map[string][]string{
		"a": {"velvet", "silent"},
		"b": {"fox", "owl", "hare"},
	})

	g := mustNew(t, WithPack(pack), WithEscalation(StepNumericSuffix(3)))
	estimate := g.EstimateCardinality()

	// 6 names x 1000 suffixes.
	closeTo(t, "Effective", estimate.Effective, 6000, 0.001)
}

func TestCardinality_UsersAt(t *testing.T) {
	estimate := Cardinality{Effective: 1_000_000}

	closeTo(t, "1%", estimate.UsersAt(0.01), 10_000, 0.001)
	closeTo(t, "10%", estimate.UsersAt(0.10), 100_000, 0.001)

	for _, rate := range []float64{0, -1, 1.5} {
		if got := estimate.UsersAt(rate); got != 0 {
			t.Errorf("UsersAt(%v) = %v, want 0 for an out-of-range rate", rate, got)
		}
	}
}

// TestEstimateCardinality_SortsByCollisionShare keeps the report readable: the
// entry to act on has to be the first one.
func TestEstimateCardinality_SortsByCollisionShare(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{"name":"p","dictionaries":{"tiny":"tiny.txt","big":"big.txt"},
		  "templates":[
		    {"pattern":["big","big"],"weight":1},
		    {"pattern":["tiny","tiny"],"weight":1}]}`,
		"tiny.txt": "aa\nbb\n",
		"big.txt":  buildWords(10),
	})

	estimate := mustNew(t, WithPack(pack), WithRepetitionCheck(false)).EstimateCardinality()
	templates := estimate.Steps[0].Templates

	for i := 1; i < len(templates); i++ {
		if templates[i-1].CollisionShare < templates[i].CollisionShare {
			t.Fatalf("templates are not sorted worst-first: %.3f before %.3f",
				templates[i-1].CollisionShare, templates[i].CollisionShare)
		}
	}
}

func TestCardinality_StringIsInformative(t *testing.T) {
	g := mustNew(t,
		WithPack(commonPack(t)),
		WithPack(cinemaPack(t)),
		WithEscalation(StepTemplateArity(2), StepNumericSuffix(3)),
	)

	report := g.EstimateCardinality().String()

	for _, want := range []string{"effective", "users at", "2-word templates", "3-digit suffix", "collisions"} {
		if !strings.Contains(report, want) {
			t.Errorf("report does not mention %q:\n%s", want, report)
		}
	}
}

// buildWords returns n distinct three-letter words, for tests that need a
// dictionary of an exact size.
func buildWords(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("w")
		b.WriteByte(byte('a' + i%26))
		b.WriteByte(byte('a' + (i/26)%26))
		b.WriteString("\n")
	}
	return b.String()
}
