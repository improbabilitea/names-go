package names_test

import (
	"math"
	"regexp"
	"strings"
	"testing"

	names "github.com/improbabilitea/names-go"
	"github.com/improbabilitea/names-go/packs/cinema"
	"github.com/improbabilitea/names-go/packs/common"
)

// These tests exercise the shipped packs through the public API only, which is
// also what makes them the check that the library is usable as published: an
// import path, two packs and a constructor.

func TestCommonPack_StandsAlone(t *testing.T) {
	g, err := names.New(names.WithPack(common.Pack))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if warnings := g.Warnings(); len(warnings) != 0 {
		t.Errorf("the shipped pack produced warnings: %v", warnings)
	}

	result, err := g.Generate(names.Seed("standalone"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Value == "" {
		t.Fatal("Generate returned an empty name")
	}
}

// TestCinemaPack_NeedsTheCommonPack pins the documented dependency. The cinema
// templates reference adjective, mood, color and animal, none of which it
// defines.
func TestCinemaPack_NeedsTheCommonPack(t *testing.T) {
	_, err := names.New(names.WithPack(cinema.Pack))

	if err == nil {
		t.Fatal("New succeeded with only the cinema pack")
	}
	if !strings.Contains(err.Error(), "cinema") {
		t.Errorf("error %q does not name the pack that is missing a dependency", err)
	}
}

func TestPacks_ComposeTogether(t *testing.T) {
	g, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if warnings := g.Warnings(); len(warnings) != 0 {
		t.Errorf("composing the shipped packs produced warnings: %v", warnings)
	}

	results, err := g.GenerateN(2000, names.Seed("compose"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}

	var cinemaNames int
	for _, r := range results {
		for _, category := range r.Metadata.Template {
			if category == "cinema_noun" {
				cinemaNames++
				break
			}
		}
	}

	share := float64(cinemaNames) / float64(len(results))
	if share < 0.75 || share > 0.85 {
		t.Errorf("cinema share = %.3f at PackWeight 1 vs 4, want about 0.80", share)
	}
}

// TestPacks_ProduceUsableUsernames is the realistic product configuration, and
// the check that the vocabulary and the constraints were curated to fit each
// other: a low rejection rate is the evidence.
func TestPacks_ProduceUsableUsernames(t *testing.T) {
	g, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
		names.WithFormat(names.Username),
		names.WithMinLength(5),
		names.WithMaxLength(20),
		names.WithCharset(names.CharsetLowerAlnum),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	valid := regexp.MustCompile(`^[a-z0-9]{5,20}$`)

	results, err := g.GenerateN(5000, names.Seed("usernames"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}

	var attempts int
	for _, r := range results {
		if !valid.MatchString(r.Value) {
			t.Fatalf("%q does not satisfy the configured policy", r.Value)
		}
		attempts += r.Metadata.Attempts
	}

	// Attempts per accepted name. Above ~1.3 means the vocabulary is fighting
	// the constraints and the rejection budget is being spent on every request.
	rate := float64(attempts) / float64(len(results))
	if rate > 1.3 {
		t.Errorf("average attempts per name = %.3f, want at most 1.30", rate)
	}
	t.Logf("average attempts per name: %.3f", rate)
}

func TestPacks_ProduceUsableSlugs(t *testing.T) {
	g, err := names.New(
		names.WithPack(common.Pack),
		names.WithFormat(names.Slug),
		names.WithMaxLength(32),
		names.WithCharset(names.CharsetSlug),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

	results, err := g.GenerateN(2000, names.Seed("slugs"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}
	for _, r := range results {
		if !valid.MatchString(r.Value) {
			t.Fatalf("%q is not a well-formed slug", r.Value)
		}
	}
}

func TestPacks_ProduceUsableDisplayNames(t *testing.T) {
	g, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(2)),
		names.WithFormat(names.DisplayName),
		names.WithMaxLength(60),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	valid := regexp.MustCompile(`^[A-Z][a-z]*( [A-Z][a-z]*)+$`)

	results, err := g.GenerateN(2000, names.Seed("display"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}
	for _, r := range results {
		if !valid.MatchString(r.Value) {
			t.Fatalf("%q is not a well-formed display name", r.Value)
		}
	}
}

// TestPacks_CardinalityIsAdequate checks the analytical estimate against
// reality.
//
// EstimateCardinality computes a number from the weights and the dictionary
// sizes; this samples the generator and infers the same number from the observed
// collision rate. They should agree, and if they diverge the formula is wrong
// rather than the vocabulary.
//
// Sampling is with replacement — GenerateN deduplicates, which would make every
// draw distinct by construction and the measurement meaningless.
func TestPacks_CardinalityIsAdequate(t *testing.T) {
	g, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const draws = 20_000

	distinct := make(map[string]struct{}, draws)
	for range draws {
		r, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		distinct[r.Value] = struct{}{}
	}

	// Collisions here are the birthday problem: drawing n times from a space of
	// size m yields about m(1 - e^(-n/m)) distinct values. Inverting that
	// recovers m from what was observed.
	observed := estimateSpace(draws, len(distinct))
	predicted := g.EstimateCardinality().Effective

	t.Logf("%d distinct in %d draws: observed space %.0f, predicted %.0f",
		len(distinct), draws, observed, predicted)

	if observed < 80_000 {
		t.Errorf("observed space is only %.0f; the vocabulary has shrunk", observed)
	}

	// Generous tolerance: the analytical figure assumes uniformity within each
	// template and ignores the filters, so it runs optimistic by design. A
	// factor of two apart means one of the two is wrong.
	if ratio := observed / predicted; ratio < 0.5 || ratio > 2.0 {
		t.Errorf("observed %.0f and predicted %.0f differ by %.2fx; the estimate has drifted from reality",
			observed, predicted, ratio)
	}
}

// estimateSpace inverts the expected-distinct-values curve to approximate how
// large the reachable name space is.
func estimateSpace(draws, distinct int) float64 {
	n, d := float64(draws), float64(distinct)
	if d >= n {
		return math.Inf(1)
	}

	// Solve d = m(1 - e^(-n/m)) for m by bisection; the function is monotonic in
	// m, so this converges quickly and needs no derivative.
	low, high := n, n*n
	for range 200 {
		mid := (low + high) / 2
		if mid*(1-math.Exp(-n/mid)) < d {
			low = mid
		} else {
			high = mid
		}
	}
	return (low + high) / 2
}

// TestPacks_ReadableSample renders a sample for a human to look at. It cannot
// fail on taste, but a curation regression is usually obvious on sight, and
// `go test -run ReadableSample -v` is where to look.
func TestPacks_ReadableSample(t *testing.T) {
	g, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	results, err := g.GenerateN(24, names.Seed("sample"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}

	var b strings.Builder
	for _, r := range results {
		b.WriteString("\n  ")
		b.WriteString(r.Value)
	}
	t.Log("sample:" + b.String())
}

// TestPacks_CardinalityMeetsTheTarget is the regression guard on the vocabulary.
// Names have to stay unique at scale, and that is a property of the word lists
// and the templates together — a curation change that quietly halves the space
// should fail here rather than in production.
//
// The two figures are deliberately separate. Without a ladder the packs are a
// weighted mixture, and the smallest template caps the whole thing; with one,
// the rungs add. The gap between the two numbers is the argument for the ladder.
func TestPacks_CardinalityMeetsTheTarget(t *testing.T) {
	base := []names.Option{
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
	}

	mixed, err := names.New(base...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	laddered, err := names.New(append(base,
		names.WithEscalation(
			names.StepTemplateArity(2, names.Attempts(40)),
			names.StepTemplateArity(3, names.Attempts(40)),
			names.StepNumericSuffix(3, names.Attempts(20)),
		),
	)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mixedEstimate := mixed.EstimateCardinality()
	ladderEstimate := laddered.EstimateCardinality()

	if mixedEstimate.Effective < 80_000 {
		t.Errorf("weighted mixture reaches %.0f effective names, want at least 80k",
			mixedEstimate.Effective)
	}
	if ladderEstimate.Effective < 200_000_000 {
		t.Errorf("with the ladder the packs reach %.0f effective names, want at least 200M",
			ladderEstimate.Effective)
	}

	// The short-name rung is what most users will actually get, so it has its
	// own floor: too small and everyone is pushed onto three-word names early.
	if short := ladderEstimate.Steps[0].Effective; short < 100_000 {
		t.Errorf("the two-word rung reaches only %.0f names, want at least 100k", short)
	}

	t.Logf("mixture: %.0f effective (%.0f users at 1%%)",
		mixedEstimate.Effective, mixedEstimate.UsersAt(0.01))
	t.Logf("ladder:  %.0f effective (%.0f users at 1%%)",
		ladderEstimate.Effective, ladderEstimate.UsersAt(0.01))
	for _, step := range ladderEstimate.Steps {
		t.Logf("  %-20s %.0f effective", step.Step, step.Effective)
	}
}
