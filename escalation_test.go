package names

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ladderPack has one two-word and one three-word template, both drawing from
// tiny dictionaries, so a test can exhaust a rung on purpose.
func ladderPack(t *testing.T) *Pack {
	t.Helper()
	return mustLoad(t, map[string]string{
		"pack.json": `{"name":"ladder","dictionaries":{"a":"a.txt","b":"b.txt","c":"c.txt"},
		  "templates":[
		    {"pattern":["a","b"],"weight":1},
		    {"pattern":["c","a","b"],"weight":1}]}`,
		"a.txt": "velvet\nsilent\n",
		"b.txt": "fox\nowl\n",
		"c.txt": "crimson\nazure\n",
	})
}

// takeAll refuses every candidate, which is what drives the ladder all the way
// down.
func takeAll(seen *[]string) CheckFunc {
	return func(_ context.Context, candidate string) (bool, error) {
		*seen = append(*seen, candidate)
		return false, nil
	}
}

// TestEscalation_DescendsThroughTheRungs is the behaviour the ladder exists for:
// short names while they last, longer ones once they do not, a suffix only at
// the end.
func TestEscalation_DescendsThroughTheRungs(t *testing.T) {
	g := mustNew(t,
		WithPack(ladderPack(t)),
		WithMaxAttempts(30),
		WithEscalation(
			StepTemplateArity(2, Attempts(10)),
			StepTemplateArity(3, Attempts(10)),
			StepNumericSuffix(2, Attempts(10)),
		),
	)

	var seen []string
	_, err := g.GenerateUnique(context.Background(), takeAll(&seen), Seed("ladder"))

	if !errors.Is(err, ErrNoValidCandidate) {
		t.Fatalf("err = %v, want the ladder to run out", err)
	}
	if len(seen) == 0 {
		t.Fatal("the check saw no candidates")
	}

	var twoWord, threeWord, suffixed int
	for _, name := range seen {
		switch {
		case strings.ContainsAny(name, "0123456789"):
			suffixed++
		case countVocabularyWords(name) == 3:
			threeWord++
		default:
			twoWord++
		}
	}

	if twoWord == 0 || threeWord == 0 || suffixed == 0 {
		t.Fatalf("expected all three rungs to be used, got %d two-word, %d three-word, %d suffixed",
			twoWord, threeWord, suffixed)
	}

	// Order matters: a suffix must never appear before the word-only rungs are
	// spent, or the ladder is not a ladder.
	firstSuffixed := -1
	lastPlain := -1
	for i, name := range seen {
		if strings.ContainsAny(name, "0123456789") {
			if firstSuffixed < 0 {
				firstSuffixed = i
			}
		} else {
			lastPlain = i
		}
	}
	if firstSuffixed < lastPlain {
		t.Errorf("a suffixed name appeared at %d, before the last plain one at %d", firstSuffixed, lastPlain)
	}
}

// countVocabularyWords counts how many of the fixture's words the name contains,
// which is how a test tells a two-word rung from a three-word one.
func countVocabularyWords(name string) int {
	count := 0
	for _, d := range []string{"velvet", "silent", "fox", "owl", "crimson", "azure"} {
		count += strings.Count(name, d)
	}
	return count
}

func TestEscalation_UsesAllTemplatesByDefault(t *testing.T) {
	g := mustNew(t, WithPack(ladderPack(t)))

	if len(g.escalation) != 1 {
		t.Fatalf("default ladder has %d rungs, want 1", len(g.escalation))
	}
	if got := len(g.escalation[0].templates); got != 2 {
		t.Errorf("the single rung covers %d templates, want 2", got)
	}
	if got := g.escalation[0].attempts; got != 100 {
		t.Errorf("the single rung has %d attempts, want the default MaxAttempts of 100", got)
	}
}

// TestEscalation_SkipsAnArityNoTemplateProvides keeps a misconfigured rung from
// becoming a runtime mystery: it is reported at construction and stepped over.
func TestEscalation_SkipsAnArityNoTemplateProvides(t *testing.T) {
	g := mustNew(t,
		WithPack(ladderPack(t)),
		WithEscalation(
			StepTemplateArity(2),
			StepTemplateArity(7),
		),
	)

	if len(g.escalation) != 1 {
		t.Fatalf("ladder has %d rungs, want the 7-word one skipped", len(g.escalation))
	}

	warnings := g.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "7-word") {
		t.Fatalf("Warnings() = %v, want one mentioning the skipped 7-word step", warnings)
	}
}

func TestEscalation_FailsWhenEveryStepIsSkipped(t *testing.T) {
	_, err := New(
		WithPack(ladderPack(t)),
		WithEscalation(StepTemplateArity(7), StepTemplateArity(8)),
	)

	if !errors.Is(err, ErrNoTemplate) {
		t.Fatalf("err = %v, want ErrNoTemplate", err)
	}
}

func TestEscalation_SharesTheAttemptBudget(t *testing.T) {
	g := mustNew(t,
		WithPack(ladderPack(t)),
		WithMaxAttempts(30),
		WithEscalation(StepTemplateArity(2), StepTemplateArity(3), StepNumericSuffix(2)),
	)

	for i, step := range g.escalation {
		if step.attempts != 10 {
			t.Errorf("rung %d (%s) got %d attempts, want 30 split three ways", i, step.label, step.attempts)
		}
	}
}

func TestEscalation_ExplicitBudgetsTakePrecedence(t *testing.T) {
	g := mustNew(t,
		WithPack(ladderPack(t)),
		WithMaxAttempts(30),
		WithEscalation(
			StepTemplateArity(2, Attempts(20)),
			StepTemplateArity(3),
		),
	)

	if got := g.escalation[0].attempts; got != 20 {
		t.Errorf("explicit rung got %d attempts, want 20", got)
	}
	if got := g.escalation[1].attempts; got != 10 {
		t.Errorf("implicit rung got %d attempts, want the remaining 10", got)
	}
}

// TestEscalation_RejectsOverbookedBudgets refuses two contradictory
// instructions rather than silently honouring one.
func TestEscalation_RejectsOverbookedBudgets(t *testing.T) {
	_, err := New(
		WithPack(ladderPack(t)),
		WithMaxAttempts(10),
		WithEscalation(StepTemplateArity(2, Attempts(8)), StepTemplateArity(3, Attempts(8))),
	)

	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "16") {
		t.Errorf("error %q does not say how much was claimed", err)
	}
}

func TestEscalation_RejectsInvalidSteps(t *testing.T) {
	cases := []struct {
		name string
		opt  Option
	}{
		{"no steps", WithEscalation()},
		{"zero arity", WithEscalation(StepTemplateArity(0))},
		{"negative arity", WithEscalation(StepTemplateArity(-1))},
		{"zero digits", WithEscalation(StepNumericSuffix(0))},
		{"too many digits", WithEscalation(StepNumericSuffix(19))},
		{"negative budget", WithEscalation(StepTemplateArity(2, Attempts(-1)))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(WithPack(ladderPack(t)), tc.opt); !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("err = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

// TestEscalation_SuffixFollowsTheFormat checks the digits go through the
// formatter as a word rather than being pasted onto the finished string.
func TestEscalation_SuffixFollowsTheFormat(t *testing.T) {
	cases := []struct {
		format Format
		want   string
	}{
		{Username, "velvetfox42"},
		{Slug, "velvet-fox-42"},
		{Snake, "velvet_fox_42"},
		{DisplayName, "Velvet Fox 42"},
	}

	pack := singleWordPack(t, "p", []string{"a", "b"}, map[string][]string{
		"a": {"velvet"},
		"b": {"fox"},
	})

	for _, tc := range cases {
		t.Run(tc.format.String(), func(t *testing.T) {
			g := mustNew(t,
				WithPack(pack),
				WithFormat(tc.format),
				WithEscalation(StepNumericSuffix(2, Attempts(5))),
			)

			result, err := g.Generate(Seed("suffix"))
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}

			// The digits themselves vary with the seed; the shape must not.
			shape := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return '4'
				}
				return r
			}, result.Value)
			want := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return '4'
				}
				return r
			}, tc.want)

			if shape != want {
				t.Errorf("Value = %q, want the shape of %q", result.Value, tc.want)
			}
		})
	}
}

func TestEscalation_SuffixIsZeroPadded(t *testing.T) {
	cases := []struct {
		value  uint64
		digits int
		want   string
	}{
		{7, 3, "007"},
		{42, 3, "042"},
		{999, 3, "999"},
		{0, 2, "00"},
		{5, 1, "5"},
		{1234, 2, "1234"}, // never truncates
	}

	for _, tc := range cases {
		if got := padDigits(tc.value, tc.digits); got != tc.want {
			t.Errorf("padDigits(%d, %d) = %q, want %q", tc.value, tc.digits, got, tc.want)
		}
	}
}

func TestEscalation_ReportsTheRungInMetadata(t *testing.T) {
	g := mustNew(t,
		WithPack(ladderPack(t)),
		WithEscalation(StepTemplateArity(2), StepTemplateArity(3)),
	)

	result, err := g.Generate(Seed("rung"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Metadata.Step != "2-word templates" {
		t.Errorf("Step = %q, want %q", result.Metadata.Step, "2-word templates")
	}
}

func TestEscalation_IsDeterministic(t *testing.T) {
	g := mustNew(t,
		WithPack(ladderPack(t)),
		WithMaxAttempts(30),
		WithEscalation(
			StepTemplateArity(2, Attempts(3)),
			StepTemplateArity(3, Attempts(3)),
			StepNumericSuffix(2, Attempts(3)),
		),
	)

	collect := func() []string {
		var seen []string
		_, _ = g.GenerateUnique(context.Background(), takeAll(&seen), Seed("determinism"))
		return seen
	}

	first := collect()
	for range 10 {
		again := collect()
		if len(again) != len(first) {
			t.Fatalf("walk length varied: %d then %d", len(first), len(again))
		}
		for i := range first {
			if again[i] != first[i] {
				t.Fatalf("candidate %d varied: %q then %q", i, first[i], again[i])
			}
		}
	}
}

// TestEscalation_WarnsWhenTheSuffixCannotFit covers the dead rung: reachable in
// the ladder but rejected by the length bound every time.
func TestEscalation_WarnsWhenTheSuffixCannotFit(t *testing.T) {
	pack := singleWordPack(t, "p", []string{"a", "b"}, map[string][]string{
		"a": {"velvet"},
		"b": {"fox"},
	})

	g := mustNew(t,
		WithPack(pack),
		WithMaxLength(9), // "velvetfox" exactly; no room for digits
		WithEscalation(StepTemplateArity(2), StepNumericSuffix(3)),
	)

	warnings := g.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "3-digit suffix") {
		t.Fatalf("Warnings() = %v, want one about the suffix not fitting", warnings)
	}
}

// TestEscalation_BatchGivesEachNameItsOwnLadder guards a subtle one: a batch is
// n independent generations, so the attempt tally must reset per name rather
// than accumulate across the call.
func TestEscalation_BatchGivesEachNameItsOwnLadder(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	// Well inside what the fixture packs can distinctly produce, so the batch
	// is not fighting its own deduplication.
	results, err := g.GenerateN(20, Seed("batch"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}

	for i, r := range results {
		if r.Metadata.Attempts > 20 {
			t.Fatalf("name %d (%q) reports %d attempts; the counter is accumulating across the batch",
				i, r.Value, r.Metadata.Attempts)
		}
	}
}
