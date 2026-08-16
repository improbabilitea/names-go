package filters_test

import (
	"strings"
	"testing"
	"testing/fstest"

	names "github.com/improbabilitea/names-go"
	"github.com/improbabilitea/names-go/filters/profanity"
	"github.com/improbabilitea/names-go/filters/reserved"
	"github.com/improbabilitea/names-go/packs/cinema"
	"github.com/improbabilitea/names-go/packs/common"
)

func fullyFiltered(t *testing.T, opts ...names.Option) *names.Generator {
	t.Helper()

	base := []names.Option{
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
		names.WithReservedWords(reserved.Default),
		names.WithTokenFilter(profanity.NewWordFilter(profanity.Words)),
		names.WithFilter(profanity.NewSubstringFilter(profanity.Substrings)),
	}

	g, err := names.New(append(base, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// TestShippedVocabularyIsClean is the check that matters most here. The word
// lists are curated, so no single word should ever trip the filters — but two
// clean words can join into something that does, and that is precisely what a
// large sample is for.
func TestShippedVocabularyIsClean(t *testing.T) {
	g := fullyFiltered(t)

	results, err := g.GenerateN(20_000, names.Seed("cleanliness"))
	if err != nil {
		t.Fatalf("GenerateN: %v", err)
	}

	var filtered int
	for _, r := range results {
		if n := r.Metadata.Rejections["profanity"]; n > 0 {
			filtered += n
		}
		if n := r.Metadata.Rejections["reserved"]; n > 0 {
			filtered += n
		}
	}

	// Not zero-tolerance: a handful of unlucky joins across 20k draws is the
	// filter earning its keep. A high rate would mean the vocabulary itself
	// needs attention.
	rate := float64(filtered) / float64(len(results))
	if rate > 0.01 {
		t.Errorf("%d filtered candidates over %d names (%.2f%%); the vocabulary needs curation, not filtering",
			filtered, len(results), rate*100)
	}
	t.Logf("%d candidates filtered over %d names", filtered, len(results))
}

func TestReservedDefault_BlocksImpersonation(t *testing.T) {
	blocked := make(map[string]bool, len(reserved.Default))
	for _, w := range reserved.Default {
		blocked[w] = true
	}

	for _, want := range []string{"admin", "support", "root", "security", "official", "api", "settings"} {
		if !blocked[want] {
			t.Errorf("reserved.Default does not contain %q", want)
		}
	}
}

func TestReservedDefault_HasNoDuplicates(t *testing.T) {
	seen := make(map[string]bool, len(reserved.Default))
	for _, w := range reserved.Default {
		if seen[w] {
			t.Errorf("reserved.Default lists %q twice", w)
		}
		seen[w] = true
	}
}

// TestReserved_MatchesThroughSeparators is what makes the list worth having: a
// filter that only matched the exact output would be sidestepped by every
// format that inserts a separator.
func TestReserved_MatchesThroughSeparators(t *testing.T) {
	pack := packOf(t, "ad", "min")

	for _, format := range []names.Format{names.Username, names.Slug, names.Snake, names.DisplayName} {
		t.Run(format.String(), func(t *testing.T) {
			g, err := names.New(
				names.WithPack(pack),
				names.WithFormat(format),
				names.WithReservedWords([]string{"admin"}),
				names.WithMaxAttempts(3),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			if _, err := g.Generate(); err == nil {
				t.Errorf("%s rendering of ad+min was not caught as %q", format, "admin")
			}
		})
	}
}

func TestProfanityWordFilter_BlocksWholeWords(t *testing.T) {
	filter := profanity.NewWordFilter([]string{"badword"})

	if filter.AllowTokens([]string{"velvet", "badword"}) {
		t.Error("a blocked word passed the token filter")
	}
	if !filter.AllowTokens([]string{"velvet", "frame"}) {
		t.Error("clean words were rejected")
	}
}

// TestProfanityWordFilter_DoesNotMatchSubstrings is the Scunthorpe guard: exact
// matching is the whole reason this list can afford to be broad.
func TestProfanityWordFilter_DoesNotMatchSubstrings(t *testing.T) {
	filter := profanity.NewWordFilter(profanity.Words)

	// Every one of these contains a listed word as a substring.
	innocent := []string{"compass", "classic", "assembly", "analysis", "cockpit", "shitake"}
	for _, word := range innocent {
		if !filter.AllowTokens([]string{word}) {
			t.Errorf("%q was rejected; the word filter is matching substrings", word)
		}
	}
}

func TestProfanitySubstringFilter_CatchesJoins(t *testing.T) {
	filter := profanity.NewSubstringFilter([]string{"cker"})

	if filter.Allow("fucker") {
		t.Error("a joined term passed the substring filter")
	}
	if !filter.Allow("velvetframe") {
		t.Error("a clean name was rejected")
	}
}

func TestProfanitySubstringFilter_NormalizesSeparators(t *testing.T) {
	filter := profanity.NewSubstringFilter([]string{"badterm"})

	for _, name := range []string{"badterm", "bad-term", "bad_term", "Bad Term", "BADTERM"} {
		if filter.Allow(name) {
			t.Errorf("%q passed; the filter is not normalizing", name)
		}
	}
}

// TestProfanitySubstrings_AreASubsetOfWords keeps the two lists coherent: a term
// severe enough to justify substring matching must also be blocked as a word.
func TestProfanitySubstrings_AreASubsetOfWords(t *testing.T) {
	words := make(map[string]bool, len(profanity.Words))
	for _, w := range profanity.Words {
		words[w] = true
	}

	for _, s := range profanity.Substrings {
		if !words[s] {
			t.Errorf("%q is matched as a substring but not listed as a word", s)
		}
	}
}

func TestProfanityFilters_ReportUnderOneName(t *testing.T) {
	word := profanity.NewWordFilter(profanity.Words)
	substring := profanity.NewSubstringFilter(profanity.Substrings)

	if word.Name() != "profanity" || substring.Name() != "profanity" {
		t.Errorf("names are %q and %q, want both %q", word.Name(), substring.Name(), "profanity")
	}
}

// packOf builds a single-candidate pack, so a test can assert on the rule rather
// than on whatever the vocabulary happened to produce.
func packOf(t *testing.T, first, second string) *names.Pack {
	t.Helper()

	files := map[string]string{
		"pack.json": `{"name":"t","dictionaries":{"a":"a.txt","b":"b.txt"},` +
			`"templates":[{"pattern":["a","b"]}]}`,
		"a.txt": first + "\n",
		"b.txt": second + "\n",
	}

	fsys := make(fstest.MapFS, len(files))
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}

	pack, err := names.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	return pack
}

func TestSampleReadsWell(t *testing.T) {
	g := fullyFiltered(t,
		names.WithFormat(names.Username),
		names.WithMinLength(5),
		names.WithMaxLength(20),
		names.WithCharset(names.CharsetLowerAlnum),
	)

	results, err := g.GenerateN(20, names.Seed("filtered-sample"))
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
