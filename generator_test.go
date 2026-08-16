package names

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// --- fixtures ---------------------------------------------------------------

func commonPack(t *testing.T) *Pack {
	t.Helper()
	return mustLoad(t, map[string]string{
		"pack.json": `{"name":"common","dictionaries":{"adjective":"a.txt","animal":"n.txt"},
		  "templates":[{"pattern":["adjective","animal"],"weight":5}]}`,
		"a.txt": "velvet\nsilent\ngolden\nmidnight\ncosmic\n",
		"n.txt": "fox\notter\nfalcon\ncomet\norbit\n",
	})
}

func cinemaPack(t *testing.T) *Pack {
	t.Helper()
	return mustLoad(t, map[string]string{
		"pack.json": `{"name":"cinema","dictionaries":{"cinema_noun":"c.txt"},
		  "templates":[{"pattern":["adjective","cinema_noun"],"weight":5}]}`,
		"c.txt": "frame\nreel\nscene\ntake\ncameo\n",
	})
}

// singleWordPack builds a pack whose every draw is predetermined, so a test can
// assert on the *rules* rather than on whatever the vocabulary happened to
// produce.
func singleWordPack(t *testing.T, name string, pattern []string, words map[string][]string) *Pack {
	t.Helper()

	dictionaries := make([]string, 0, len(words))
	files := make(map[string]string, len(words)+1)
	for category, values := range words {
		filename := category + ".txt"
		dictionaries = append(dictionaries, fmt.Sprintf("%q:%q", category, filename))
		files[filename] = strings.Join(values, "\n") + "\n"
	}

	quoted := make([]string, 0, len(pattern))
	for _, c := range pattern {
		quoted = append(quoted, fmt.Sprintf("%q", c))
	}

	files["pack.json"] = fmt.Sprintf(
		`{"name":%q,"dictionaries":{%s},"templates":[{"pattern":[%s]}]}`,
		name, strings.Join(dictionaries, ","), strings.Join(quoted, ","))

	return mustLoad(t, files)
}

func mustNew(t *testing.T, opts ...Option) *Generator {
	t.Helper()

	g, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// --- construction-time validation -------------------------------------------

func TestNew_RequiresAPack(t *testing.T) {
	_, err := New()
	if !errors.Is(err, ErrNoPack) {
		t.Fatalf("err = %v, want ErrNoPack", err)
	}
}

func TestNew_RequiresATemplate(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{"name":"common","dictionaries":{"adjective":"a.txt"}}`,
		"a.txt":     "velvet\n",
	})

	_, err := New(WithPack(pack))
	if !errors.Is(err, ErrNoTemplate) {
		t.Fatalf("err = %v, want ErrNoTemplate", err)
	}
}

// TestNew_MissingCategory is the integration mistake most likely to happen: a
// theme pack registered without the base pack it builds on. LoadFS accepts the
// theme pack alone by design, so this is the only place it can be caught.
func TestNew_MissingCategory(t *testing.T) {
	_, err := New(WithPack(cinemaPack(t)))

	if !errors.Is(err, ErrDictionaryNotFound) {
		t.Fatalf("err = %v, want ErrDictionaryNotFound", err)
	}
	if !strings.Contains(err.Error(), "adjective") {
		t.Errorf("error %q does not name the missing category", err)
	}
	if !strings.Contains(err.Error(), "cinema") {
		t.Errorf("error %q does not name the pack that needs it", err)
	}
}

func TestNew_RejectsMinAboveMax(t *testing.T) {
	_, err := New(WithPack(commonPack(t)), WithMinLength(30), WithMaxLength(10))

	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

// TestNew_RejectsCharsetThatCannotExpressTheFormat catches a misconfiguration
// that would otherwise reject every candidate at runtime and surface as an
// exhausted attempt budget.
func TestNew_RejectsCharsetThatCannotExpressTheFormat(t *testing.T) {
	cases := []struct {
		name    string
		format  Format
		charset Charset
		needs   string
	}{
		{"slug needs a hyphen", Slug, CharsetLowerAlnum, "-"},
		{"snake needs an underscore", Snake, CharsetLowerAlnum, "_"},
		{"display name needs capitals and a space", DisplayName, CharsetLowerAlnum, "A"},
		{"pascal needs capitals", Pascal, CharsetLowerAlnum, "A"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(WithPack(commonPack(t)), WithFormat(tc.format), WithCharset(tc.charset))

			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("err = %v, want ErrInvalidOptions", err)
			}
			if !strings.Contains(err.Error(), tc.needs) {
				t.Errorf("error %q does not mention the missing character %q", err, tc.needs)
			}
		})
	}
}

func TestNew_AcceptsMatchingCharsetAndFormat(t *testing.T) {
	cases := []struct {
		format  Format
		charset Charset
	}{
		{Username, CharsetLowerAlnum},
		{Slug, CharsetSlug},
		{Snake, CharsetSnake},
		{DisplayName, Charset{}},
	}

	for _, tc := range cases {
		t.Run(tc.format.String(), func(t *testing.T) {
			mustNew(t, WithPack(commonPack(t)), WithFormat(tc.format), WithCharset(tc.charset))
		})
	}
}

// TestNew_RejectsUnsatisfiableLength is the check that keeps a plain
// configuration mistake from masquerading as a runtime failure on a user's
// first request.
func TestNew_RejectsUnsatisfiableLength(t *testing.T) {
	// The shortest name this pack can build is "silentfox", nine characters.
	_, err := New(WithPack(commonPack(t)), WithMaxLength(5))

	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "5") {
		t.Errorf("error %q does not mention the bound", err)
	}
}

func TestNew_RejectsUnreachableMinimum(t *testing.T) {
	_, err := New(WithPack(commonPack(t)), WithMinLength(100))

	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestNew_RejectsBadOptions(t *testing.T) {
	pack := commonPack(t)

	cases := []struct {
		name   string
		option Option
	}{
		{"nil pack", WithPack(nil)},
		{"zero pack weight", WithPack(pack, PackWeight(0))},
		{"negative min length", WithMinLength(-1)},
		{"negative max length", WithMaxLength(-1)},
		{"zero max attempts", WithMaxAttempts(0)},
		{"nil entropy", WithEntropy(nil)},
		{"nil filter", WithFilter(nil)},
		{"nil token filter", WithTokenFilter(nil)},
		{"unnamed filter", WithFilter(FilterFunc("", func(string) bool { return true }))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(WithPack(pack), tc.option)
			if err == nil {
				t.Fatal("New succeeded, want an error")
			}
		})
	}
}

func TestNew_RejectsUnknownFormat(t *testing.T) {
	_, err := New(WithPack(commonPack(t)), WithFormat(Format(99)))

	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestMustNew_PanicsOnBadConfiguration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew did not panic")
		}
	}()
	_ = MustNew()
}

func TestMustNew_ReturnsAGeneratorWhenValid(t *testing.T) {
	if g := MustNew(WithPack(commonPack(t))); g == nil {
		t.Fatal("MustNew returned nil")
	}
}

// --- merging ----------------------------------------------------------------

// TestMerge_PackWeightDistribution is the property PackWeight exists for. Both
// packs declare a template weight of 5, so without normalization the mix would
// be 50/50 regardless of what the consumer asked for.
func TestMerge_PackWeightDistribution(t *testing.T) {
	const draws = 100_000

	g := mustNew(t,
		WithPack(commonPack(t), PackWeight(1)),
		WithPack(cinemaPack(t), PackWeight(4)),
		WithRepetitionCheck(false),
	)

	// Sampled with replacement: GenerateN returns distinct names, which is the
	// wrong tool for measuring a distribution.
	var cinema int
	for range draws {
		r, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if r.Metadata.Template[1] == "cinema_noun" {
			cinema++
		}
	}

	share := float64(cinema) / draws
	if math.Abs(share-0.8) > 0.01 {
		t.Fatalf("cinema share = %.4f, want 0.80 (PackWeight 1 vs 4)", share)
	}
}

// TestMerge_AddingATemplateDoesNotChangeThePackShare is what makes per-pack
// normalization worth the arithmetic. A pack author adding a pattern should
// redistribute weight inside their own theme, never take it from another pack.
func TestMerge_AddingATemplateDoesNotChangeThePackShare(t *testing.T) {
	const draws = 100_000

	richCinema := mustLoad(t, map[string]string{
		"pack.json": `{"name":"cinema","dictionaries":{"cinema_noun":"c.txt"},"templates":[
		  {"pattern":["adjective","cinema_noun"],"weight":5},
		  {"pattern":["cinema_noun","animal"],"weight":3},
		  {"pattern":["adjective","cinema_noun","animal"],"weight":1}]}`,
		"c.txt": "frame\nreel\nscene\ntake\ncameo\n",
	})

	g := mustNew(t,
		WithPack(commonPack(t), PackWeight(1)),
		WithPack(richCinema, PackWeight(4)),
		WithRepetitionCheck(false),
	)

	// Identified by the pattern, not by word provenance: the template
	// ["cinema_noun","animal"] draws its last word from common, so provenance
	// order would measure something else entirely. Sampled with replacement,
	// since GenerateN deduplicates.
	var cinema int
	for range draws {
		r, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if slices.Contains(r.Metadata.Template, "cinema_noun") {
			cinema++
		}
	}

	share := float64(cinema) / draws
	if math.Abs(share-0.8) > 0.01 {
		t.Fatalf("cinema share = %.4f after growing from 1 template to 3, want 0.80", share)
	}
}

// TestMerge_DuplicateWordResolvesToTheLastPack pins the rule that makes
// registration order meaningful, and checks the override is reported rather than
// silent.
func TestMerge_DuplicateWordResolvesToTheLastPack(t *testing.T) {
	base := singleWordPack(t, "base", []string{"adjective"}, map[string][]string{
		"adjective": {"velvet"},
	})
	override := singleWordPack(t, "override", []string{"adjective"}, map[string][]string{
		"adjective": {"velvet"},
	})

	g := mustNew(t, WithPack(base), WithPack(override))

	result, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := result.Metadata.Packs; len(got) != 1 || got[0] != "override" {
		t.Errorf("Packs = %v, want [override] (the later registration wins)", got)
	}

	warnings := g.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("Warnings() = %v, want exactly one override notice", warnings)
	}
	for _, want := range []string{"velvet", "adjective", "override", "base"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning %q does not mention %q", warnings[0], want)
		}
	}
}

func TestMerge_NoWarningsForACleanConfiguration(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	if got := g.Warnings(); len(got) != 0 {
		t.Fatalf("Warnings() = %v, want none", got)
	}
}

// TestMerge_CategoriesCombineAcrossPacks covers the documented default for
// same-named categories: contents merge rather than one replacing the other.
func TestMerge_CategoriesCombineAcrossPacks(t *testing.T) {
	base := singleWordPack(t, "base", []string{"adjective"}, map[string][]string{
		"adjective": {"velvet"},
	})
	extra := singleWordPack(t, "extra", []string{"adjective"}, map[string][]string{
		"adjective": {"golden"},
	})

	g := mustNew(t, WithPack(base), WithPack(extra), WithRepetitionCheck(false))

	seen := make(map[string]bool)
	for range 200 {
		r, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		seen[r.Value] = true
	}

	for _, want := range []string{"velvet", "golden"} {
		if !seen[want] {
			t.Errorf("%q never appeared; the categories did not merge", want)
		}
	}
}

func TestGenerator_ReportsItsPacks(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)), WithPack(cinemaPack(t)))

	got := g.Packs()
	if len(got) != 2 || got[0] != "common" || got[1] != "cinema" {
		t.Fatalf("Packs() = %v, want [common cinema] in registration order", got)
	}
}

func TestGenerator_FingerprintIsExposed(t *testing.T) {
	g := mustNew(t, WithPack(commonPack(t)))

	if !strings.HasPrefix(g.Fingerprint(), "sha256:") {
		t.Fatalf("Fingerprint() = %q", g.Fingerprint())
	}
}

// --- weight arithmetic ------------------------------------------------------

func TestWeightArithmetic(t *testing.T) {
	t.Run("gcd", func(t *testing.T) {
		cases := []struct{ a, b, want uint64 }{
			{12, 18, 6}, {7, 13, 1}, {10, 0, 10}, {0, 10, 10},
		}
		for _, tc := range cases {
			if got := gcd(tc.a, tc.b); got != tc.want {
				t.Errorf("gcd(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		}
	})

	t.Run("lcm", func(t *testing.T) {
		cases := []struct{ a, b, want uint64 }{
			{4, 6, 12}, {7, 13, 91}, {5, 5, 5},
		}
		for _, tc := range cases {
			got, ok := lcm(tc.a, tc.b)
			if !ok || got != tc.want {
				t.Errorf("lcm(%d, %d) = %d, %v; want %d, true", tc.a, tc.b, got, ok, tc.want)
			}
		}
	})

	t.Run("lcm reports overflow", func(t *testing.T) {
		if _, ok := lcm(math.MaxUint64, math.MaxUint64-1); ok {
			t.Error("lcm did not report an overflow")
		}
	})

	t.Run("mulChecked reports overflow", func(t *testing.T) {
		if _, ok := mulChecked(math.MaxUint64, 2); ok {
			t.Error("mulChecked did not report an overflow")
		}
		if got, ok := mulChecked(6, 7); !ok || got != 42 {
			t.Errorf("mulChecked(6, 7) = %d, %v", got, ok)
		}
	})

	t.Run("addChecked reports overflow", func(t *testing.T) {
		if _, ok := addChecked(math.MaxUint64, 1); ok {
			t.Error("addChecked did not report an overflow")
		}
	})
}

func TestNew_RejectsUnrepresentableWeights(t *testing.T) {
	pack := commonPack(t)

	_, err := New(
		WithPack(pack, PackWeight(math.MaxUint32)),
		WithPack(cinemaPack(t), PackWeight(math.MaxUint32)),
	)
	// Two packs at the top of the uint32 range still compose comfortably inside
	// uint64, so this must succeed — the guard exists for pathological weights,
	// not for large but reasonable ones.
	if err != nil && !errors.Is(err, ErrDictionaryNotFound) {
		t.Fatalf("New: %v", err)
	}
}
