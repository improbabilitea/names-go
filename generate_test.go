package names

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// --- determinism ------------------------------------------------------------

func TestGenerate_SeedIsStable(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	first, err := g.Generate(Seed("user_01893"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for range 50 {
		again, err := g.Generate(Seed("user_01893"))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if again.Value != first.Value {
			t.Fatalf("same seed produced %q then %q", first.Value, again.Value)
		}
	}
}

// TestGenerate_SeedIsStableAcrossRetries is the case a naive implementation gets
// wrong. A candidate that fails a filter advances the stream, so if the source
// were shared across calls rather than created fresh per call, the second
// Generate with the same seed would start from a different position.
func TestGenerate_SeedIsStableAcrossRetries(t *testing.T) {
	// Half the vocabulary is too long, so most attempts are rejected.
	pack := singleWordPack(t, "common", []string{"adjective", "animal"}, map[string][]string{
		"adjective": {"red", "silver", "midnight", "extraordinary"},
		"animal":    {"fox", "falcon", "chinchilla", "hippopotamus"},
	})
	g := mustNew(t, WithPack(pack), WithMaxLength(9))

	first, err := g.Generate(Seed("retries"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if first.Metadata.Attempts < 2 {
		t.Skip("this seed happened not to need a retry; the test proves nothing")
	}

	for range 20 {
		again, err := g.Generate(Seed("retries"))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if again.Value != first.Value || again.Metadata.Attempts != first.Metadata.Attempts {
			t.Fatalf("after %d retries the seed produced %q(%d) then %q(%d)",
				first.Metadata.Attempts, first.Value, first.Metadata.Attempts,
				again.Value, again.Metadata.Attempts)
		}
	}
}

func TestGenerate_DifferentSeedsDiverge(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	seen := make(map[string]bool)
	for i := range 20 {
		result, err := g.Generate(Seed(fmt.Sprintf("user_%d", i)))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		seen[result.Value] = true
	}

	if len(seen) < 5 {
		t.Fatalf("20 distinct seeds produced only %d distinct names", len(seen))
	}
}

func TestGenerate_UnseededVaries(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	seen := make(map[string]bool)
	for range 50 {
		result, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		seen[result.Value] = true
	}

	if len(seen) < 5 {
		t.Fatalf("50 unseeded calls produced only %d distinct names", len(seen))
	}
}

// TestGenerate_SeedWinsOverEntropy documents the precedence: a caller who passed
// a seed is asking for reproducibility, and honouring a configured crypto source
// instead would quietly break it.
func TestGenerate_SeedWinsOverEntropy(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithEntropy(CryptoEntropy))

	first, err := g.Generate(Seed("fixed"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for range 20 {
		again, _ := g.Generate(Seed("fixed"))
		if again.Value != first.Value {
			t.Fatalf("a seed did not override the crypto entropy source: %q then %q",
				first.Value, again.Value)
		}
	}
}

func TestGenerate_CryptoEntropyProducesNames(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithEntropy(CryptoEntropy))

	seen := make(map[string]bool)
	for range 50 {
		result, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		seen[result.Value] = true
	}
	if len(seen) < 5 {
		t.Fatalf("crypto entropy produced only %d distinct names in 50 calls", len(seen))
	}
}

// --- formatting -------------------------------------------------------------

func TestGenerate_FormatsTheSameLogicalName(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	base, err := g.Generate(Seed("format"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	cases := []struct {
		format Format
		want   string
	}{
		{Username, strings.Join(base.Words, "")},
		{Slug, strings.Join(base.Words, "-")},
		{Snake, strings.Join(base.Words, "_")},
	}

	for _, tc := range cases {
		t.Run(tc.format.String(), func(t *testing.T) {
			result, err := g.Generate(Seed("format"), UseFormat(tc.format))
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if result.Value != tc.want {
				t.Errorf("Value = %q, want %q", result.Value, tc.want)
			}
			// The same seed must select the same words whatever the rendering.
			if strings.Join(result.Words, ",") != strings.Join(base.Words, ",") {
				t.Errorf("Words = %v, want %v", result.Words, base.Words)
			}
		})
	}
}

func TestGenerate_CapitalizingFormats(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithFormat(DisplayName))

	result, err := g.Generate(Seed("caps"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, word := range strings.Fields(result.Value) {
		if word == "" || !strings.HasPrefix(word, strings.ToUpper(word[:1])) {
			t.Errorf("word %q in %q is not capitalized", word, result.Value)
		}
	}
	if strings.Count(result.Value, " ") != len(result.Words)-1 {
		t.Errorf("Value = %q, want %d words separated by spaces", result.Value, len(result.Words))
	}
}

func TestGenerate_RejectsUnknownFormatOverride(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	_, err := g.Generate(UseFormat(Format(99)))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

// --- filters ----------------------------------------------------------------

// TestGenerate_LengthAppliesAfterFormatting is the reason the length filter runs
// post-format: a slug carries separators that count toward the limit, so the
// same word pair can fit as a username and overflow as a slug.
func TestGenerate_LengthAppliesAfterFormatting(t *testing.T) {
	pack := singleWordPack(t, "common", []string{"adjective", "animal"}, map[string][]string{
		"adjective": {"red", "silver"},
		"animal":    {"fox", "falcon"},
	})

	collect := func(format Format) map[string]bool {
		g := mustNew(t, WithPack(pack), WithFormat(format), WithMaxLength(9))
		seen := make(map[string]bool)
		for i := range 200 {
			result, err := g.Generate(Seed(fmt.Sprintf("%d", i)))
			if err != nil {
				continue
			}
			seen[result.Value] = true
		}
		return seen
	}

	usernames, slugs := collect(Username), collect(Slug)

	// "silver" + "fox" is 9 characters joined, 10 with a hyphen.
	if !usernames["silverfox"] {
		t.Error(`"silverfox" (9 characters) should be a valid username at MaxLength 9`)
	}
	if slugs["silver-fox"] {
		t.Error(`"silver-fox" (10 characters) should be rejected as a slug at MaxLength 9`)
	}
	if !slugs["red-fox"] {
		t.Error(`"red-fox" (7 characters) should be a valid slug at MaxLength 9`)
	}
}

func TestGenerate_RepetitionIsRejected(t *testing.T) {
	cases := []struct {
		name  string
		words map[string][]string
		joins string
	}{
		{
			name:  "the same word twice",
			words: map[string][]string{"adjective": {"silent"}},
			joins: "silentsilent",
		},
		{
			name:  "a shared seam",
			words: map[string][]string{"adjective": {"golden"}, "animal": {"denim"}},
			joins: "goldendenim",
		},
		{
			name:  "a doubled letter at the join",
			words: map[string][]string{"adjective": {"silent"}, "animal": {"take"}},
			joins: "silenttake",
		},
		{
			name:  "one word contained in its neighbour",
			words: map[string][]string{"adjective": {"silent"}, "animal": {"silently"}},
			joins: "silentsilently",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pattern := []string{"adjective", "adjective"}
			if len(tc.words) > 1 {
				pattern = []string{"adjective", "animal"}
			}
			pack := singleWordPack(t, "common", pattern, tc.words)

			g := mustNew(t, WithPack(pack), WithMaxAttempts(10))
			_, err := g.Generate(Seed("repetition"))

			var detail *NoValidCandidateError
			if !errors.As(err, &detail) {
				t.Fatalf("Generate produced %v, want every candidate rejected", err)
			}
			if got := detail.Rejections["repetition"]; got != 10 {
				t.Errorf("repetition rejections = %d, want 10 (%q should never be accepted)",
					got, tc.joins)
			}
		})
	}
}

func TestGenerate_RepetitionCanBeDisabled(t *testing.T) {
	pack := singleWordPack(t, "common", []string{"adjective", "adjective"}, map[string][]string{
		"adjective": {"silent"},
	})

	g := mustNew(t, WithPack(pack), WithRepetitionCheck(false))

	result, err := g.Generate(Seed("repetition"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Value != "silentsilent" {
		t.Fatalf("Value = %q, want %q", result.Value, "silentsilent")
	}
}

// TestGenerate_ReservedWordOnTokens covers the case where a single selected word
// is reserved even though the joined name is not.
func TestGenerate_ReservedWordOnTokens(t *testing.T) {
	pack := singleWordPack(t, "common", []string{"adjective", "animal"}, map[string][]string{
		"adjective": {"support"},
		"animal":    {"fox"},
	})

	g := mustNew(t, WithPack(pack), WithReservedWords([]string{"support"}), WithMaxAttempts(5))
	_, err := g.Generate(Seed("reserved"))

	var detail *NoValidCandidateError
	if !errors.As(err, &detail) {
		t.Fatalf("Generate produced %v, want a rejection", err)
	}
	if detail.Rejections["reserved"] != 5 {
		t.Errorf("Rejections = %v, want 5 reserved rejections", detail.Rejections)
	}
}

// TestGenerate_ReservedWordOnNormalizedOutput is the subtler half: neither word
// is reserved on its own, and the reservation only bites once they are joined —
// and it has to bite through the separator a slug inserts.
func TestGenerate_ReservedWordOnNormalizedOutput(t *testing.T) {
	// "ad" and "min" do not collide at the seam, so the repetition filter lets
	// them through and only the reserved check can reject "ad-min".
	pack := singleWordPack(t, "common", []string{"adjective", "animal"}, map[string][]string{
		"adjective": {"ad"},
		"animal":    {"min"},
	})

	g := mustNew(t,
		WithPack(pack),
		WithFormat(Slug),
		WithReservedWords([]string{"admin"}),
		WithMaxAttempts(5),
	)
	_, err := g.Generate(Seed("reserved"))

	var detail *NoValidCandidateError
	if !errors.As(err, &detail) {
		t.Fatalf(`Generate produced %v, want "ad-min" rejected as "admin"`, err)
	}
	if detail.Rejections["reserved"] != 5 {
		t.Errorf("Rejections = %v, want 5 reserved rejections", detail.Rejections)
	}
}

func TestGenerate_CustomStringFilter(t *testing.T) {
	g := mustNew(t,
		WithPack(commonPack(t)),
		WithFilter(FilterFunc("no-golden", func(name string) bool {
			return !strings.HasPrefix(name, "golden")
		})),
	)

	for i := range 200 {
		result, err := g.Generate(Seed(fmt.Sprintf("%d", i)))
		if err != nil {
			continue
		}
		if strings.HasPrefix(result.Value, "golden") {
			t.Fatalf("custom filter did not run: %q", result.Value)
		}
	}
}

type noVelvet struct{}

func (noVelvet) Name() string { return "no-velvet" }
func (noVelvet) AllowTokens(words []string) bool {
	for _, w := range words {
		if w == "velvet" {
			return false
		}
	}
	return true
}

func TestGenerate_CustomTokenFilter(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithTokenFilter(noVelvet{}))

	for i := range 200 {
		result, err := g.Generate(Seed(fmt.Sprintf("%d", i)))
		if err != nil {
			continue
		}
		for _, w := range result.Words {
			if w == "velvet" {
				t.Fatalf("custom token filter did not run: %v", result.Words)
			}
		}
	}
}

// --- failure ----------------------------------------------------------------

func TestGenerate_NoValidCandidateCarriesTheBreakdown(t *testing.T) {
	pack := singleWordPack(t, "common", []string{"adjective", "animal"}, map[string][]string{
		"adjective": {"extraordinary"},
		"animal":    {"hippopotamus"},
	})

	// 13 + 12 = 25 characters, so the length bound is satisfiable; the reserved
	// list is what rejects every candidate.
	g := mustNew(t, WithPack(pack), WithMaxLength(26), WithMaxAttempts(7),
		WithReservedWords([]string{"extraordinaryhippopotamus"}))

	_, err := g.Generate()

	if !errors.Is(err, ErrNoValidCandidate) {
		t.Fatalf("err = %v, want ErrNoValidCandidate", err)
	}

	var detail *NoValidCandidateError
	if !errors.As(err, &detail) {
		t.Fatalf("err = %v, want a *NoValidCandidateError", err)
	}
	if detail.Attempts != 7 {
		t.Errorf("Attempts = %d, want 7", detail.Attempts)
	}
	if detail.Rejections["reserved"] != 7 {
		t.Errorf("Rejections = %v, want 7 reserved", detail.Rejections)
	}
	if !strings.Contains(err.Error(), "reserved=7") {
		t.Errorf("message %q does not carry the breakdown", err)
	}
}

// TestGenerate_FailureIsDeterministic documents the consequence of seeded
// generation: an exhausted budget under a fixed seed is permanent, not
// transient. Retrying at the call site will fail identically.
func TestGenerate_FailureIsDeterministic(t *testing.T) {
	pack := singleWordPack(t, "common", []string{"adjective", "animal"}, map[string][]string{
		"adjective": {"red", "silver"},
		"animal":    {"fox", "falcon"},
	})
	g := mustNew(t, WithPack(pack), WithMaxLength(6), WithMaxAttempts(20))

	var messages []string
	for range 10 {
		_, err := g.Generate(Seed("doomed"))
		if err == nil {
			t.Skip("this configuration happened to succeed; the test proves nothing")
		}
		messages = append(messages, err.Error())
	}

	for i, m := range messages {
		if m != messages[0] {
			t.Fatalf("failure %d differed: %q vs %q", i, m, messages[0])
		}
	}
}

// --- metadata ---------------------------------------------------------------

func TestGenerate_MetadataIsPopulated(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	result, err := g.Generate(Seed("metadata"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	m := result.Metadata
	if m.Attempts < 1 {
		t.Errorf("Attempts = %d", m.Attempts)
	}
	if len(m.Template) != len(result.Words) {
		t.Errorf("Template = %v, does not match Words = %v", m.Template, result.Words)
	}
	if len(m.Packs) == 0 {
		t.Error("Packs is empty")
	}
	if m.Fingerprint != g.Fingerprint() {
		t.Errorf("Fingerprint = %q, want %q", m.Fingerprint, g.Fingerprint())
	}
}

// TestGenerate_RejectionsAreNilWhenNothingWasRejected keeps the happy path from
// allocating a map it has nothing to put in.
func TestGenerate_RejectionsAreNilWhenNothingWasRejected(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	for i := range 100 {
		result, err := g.Generate(Seed(fmt.Sprintf("%d", i)))
		if err != nil {
			continue
		}
		if result.Metadata.Attempts == 1 && result.Metadata.Rejections != nil {
			t.Fatalf("first-attempt success carries Rejections = %v", result.Metadata.Rejections)
		}
	}
}

// --- batch and uniqueness ---------------------------------------------------

func TestGenerateN(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	results, err := g.GenerateN(25)
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}
	if len(results) != 25 {
		t.Fatalf("got %d results, want 25", len(results))
	}
	for i, r := range results {
		if r.Value == "" {
			t.Fatalf("result %d is empty", i)
		}
	}
}

func TestGenerateN_RejectsNonPositiveCounts(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	for _, n := range []int{0, -1} {
		if _, err := g.GenerateN(n); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("GenerateN(%d) error = %v, want ErrInvalidOptions", n, err)
		}
	}
}

// TestGenerateN_SeededSequenceIsReproducibleAndVaried checks both halves of the
// documented behaviour: the sequence repeats exactly, and it is a sequence
// rather than the same name n times.
func TestGenerateN_SeededSequenceIsReproducibleAndVaried(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	first, err := g.GenerateN(20, Seed("sequence"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}
	second, err := g.GenerateN(20, Seed("sequence"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}

	distinct := make(map[string]bool)
	for i := range first {
		if first[i].Value != second[i].Value {
			t.Fatalf("position %d: %q then %q", i, first[i].Value, second[i].Value)
		}
		distinct[first[i].Value] = true
	}

	if len(distinct) < 5 {
		t.Fatalf("a seeded batch of 20 produced only %d distinct names", len(distinct))
	}
}

func TestGenerateUnique(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	taken := map[string]bool{}
	var offered []string

	// Refuses the first three *distinct* candidates and stays consistent about
	// them afterwards, which is what a datastore does. Refusing by position
	// instead would accept a value it had already rejected the moment the
	// generator offered it twice — and with a small vocabulary it does, since
	// GenerateUnique deliberately does not deduplicate.
	check := func(_ context.Context, candidate string) (bool, error) {
		offered = append(offered, candidate)
		if taken[candidate] {
			return false, nil
		}
		if len(taken) < 3 {
			taken[candidate] = true
			return false, nil
		}
		return true, nil
	}

	result, err := g.GenerateUnique(context.Background(), check, Seed("unique"))
	if err != nil {
		t.Fatalf("GenerateUnique: %v", err)
	}
	if taken[result.Value] {
		t.Fatalf("returned %q, which the check had refused", result.Value)
	}
	if len(taken) != 3 {
		t.Errorf("the check refused %d distinct candidates, want 3", len(taken))
	}
	// At least four: three refused plus the one returned. It can be more, since
	// a filter rejection consumes an attempt without reaching the check, and a
	// repeat reaches the check without being new.
	if result.Metadata.Attempts < 4 {
		t.Errorf("Attempts = %d, want at least 4", result.Metadata.Attempts)
	}
	if len(offered) < 4 {
		t.Errorf("the check saw %d candidates, want at least 4", len(offered))
	}
}

func TestGenerateUnique_GivesUpAfterMaxAttempts(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithMaxAttempts(5))

	var offered int
	never := func(context.Context, string) (bool, error) {
		offered++
		return false, nil
	}

	// Seeded: without one, how many candidates reach the check depends on how
	// many the filters happened to reject first, and the assertion below would
	// be flaky.
	_, err := g.GenerateUnique(context.Background(), never, Seed("exhaust"))
	if !errors.Is(err, ErrNoValidCandidate) {
		t.Fatalf("err = %v, want ErrNoValidCandidate", err)
	}

	var detail *NoValidCandidateError
	if !errors.As(err, &detail) {
		t.Fatalf("err = %v, want a *NoValidCandidateError", err)
	}
	if detail.Attempts != 5 {
		t.Errorf("Attempts = %d, want the full budget of 5", detail.Attempts)
	}
	// Every candidate the check saw is counted as taken; the rest were rejected
	// by filters and never reached it. Together they exhaust the budget.
	if got := detail.Rejections["taken"]; got != offered {
		t.Errorf("Rejections[taken] = %d, but the check saw %d candidates", got, offered)
	}
	if offered == 0 {
		t.Error("the check never saw a candidate")
	}
}

func TestGenerateUnique_PropagatesCheckErrors(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	sentinel := errors.New("datastore unreachable")
	failing := func(context.Context, string) (bool, error) { return false, sentinel }

	_, err := g.GenerateUnique(context.Background(), failing)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the check's error", err)
	}
}

func TestGenerateUnique_HonoursContextCancellation(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := g.GenerateUnique(ctx, func(context.Context, string) (bool, error) { return true, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestGenerateUnique_RequiresACheck(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	if _, err := g.GenerateUnique(context.Background(), nil); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

// --- concurrency ------------------------------------------------------------

// TestGenerate_IsSafeForConcurrentUse is the claim that lets a process build one
// Generator and share it. Meaningful under -race, which CI runs.
func TestGenerate_IsSafeForConcurrentUse(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	for worker := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				var err error
				if i%2 == 0 {
					_, err = g.Generate()
				} else {
					_, err = g.Generate(Seed(fmt.Sprintf("w%d-%d", worker, i)))
				}
				if err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent Generate: %v", err)
	}
}

// --- benchmarks -------------------------------------------------------------

func BenchmarkGenerate(b *testing.B) {
	t := &testing.T{}
	g := MustNew(WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := g.Generate(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateSeeded(b *testing.B) {
	t := &testing.T{}
	g := MustNew(WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := g.Generate(Seed("benchmark")); err != nil {
			b.Fatal(err)
		}
	}
}

// TestGenerateN_ReturnsDistinctNames covers what a batch is for: a set of
// alternatives to choose between. The same name twice is never a useful answer.
func TestGenerateN_ReturnsDistinctNames(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	for round := range 50 {
		results, err := g.GenerateN(12, Seed(fmt.Sprintf("batch-%d", round)))
		if err != nil {
			t.Fatalf("GenerateN: %v", err)
		}

		seen := make(map[string]bool, len(results))
		for _, r := range results {
			if seen[r.Value] {
				t.Fatalf("round %d returned %q twice", round, r.Value)
			}
			seen[r.Value] = true
		}
	}
}

// TestGenerateN_ExhaustsRatherThanRepeating pins the trade the deduplication
// makes: asking for more names than exist is an error, not a list padded with
// duplicates.
func TestGenerateN_ExhaustsRatherThanRepeating(t *testing.T) {
	// Two adjectives and two animals is four names, minus what the repetition
	// filter removes.
	pack := singleWordPack(t, "tiny", []string{"a", "b"}, map[string][]string{
		"a": {"velvet", "silent"},
		"b": {"fox", "owl"},
	})
	g := mustNew(t, WithPack(pack), WithMaxAttempts(50))

	if _, err := g.GenerateN(4, Seed("small")); err != nil {
		t.Fatalf("GenerateN(4) over a four-name space: %v", err)
	}

	_, err := g.GenerateN(5, Seed("small"))
	if !errors.Is(err, ErrNoValidCandidate) {
		t.Fatalf("GenerateN(5) error = %v, want ErrNoValidCandidate", err)
	}

	var detail *NoValidCandidateError
	if errors.As(err, &detail) && detail.Rejections["duplicate"] == 0 {
		t.Errorf("Rejections = %v, want duplicates counted", detail.Rejections)
	}
}

// TestGenerateN_DiffersFromRepeatedGenerate is the gotcha worth a test: a seed
// pins one call, so ten seeded calls give ten identical names while one seeded
// batch gives ten different ones.
func TestGenerateN_DiffersFromRepeatedGenerate(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	var looped []string
	for range 10 {
		result, err := g.Generate(Seed("same"))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		looped = append(looped, result.Value)
	}
	for i, name := range looped {
		if name != looped[0] {
			t.Fatalf("call %d gave %q, want the same %q every time", i, name, looped[0])
		}
	}

	batch, err := g.GenerateN(10, Seed("same"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}
	distinct := make(map[string]bool, len(batch))
	for _, r := range batch {
		distinct[r.Value] = true
	}
	if len(distinct) != 10 {
		t.Fatalf("the batch produced %d distinct names, want 10", len(distinct))
	}
}
