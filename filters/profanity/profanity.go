// Package profanity blocks offensive names.
//
// It lives outside the core on purpose. A serious profanity list is
// language-specific, sizeable, and has a lifecycle of its own; keeping it here
// means it reaches a binary only when a consumer imports it, and can be replaced
// wholesale without touching the engine.
//
// # Two stages, two lists, on purpose
//
// Substring matching over a whole name is what catches the case that matters —
// two innocuous words that form something unwanted once joined — and it is also
// what produces false positives. The classic is the Scunthorpe problem: a
// perfectly ordinary place name contains a slur as a substring, and a naive
// filter rejects it.
//
// So the two stages carry different lists:
//
//   - Words is matched exactly, against each selected word. No false positives
//     are possible, so this list can be broad.
//   - Substrings is matched anywhere in the joined name. Every entry costs
//     false positives, so this list stays deliberately short and holds only
//     terms severe enough to be worth the collateral.
//
// Install both:
//
//	names.WithTokenFilter(profanity.NewWordFilter(profanity.Words)),
//	names.WithFilter(profanity.NewSubstringFilter(profanity.Substrings)),
//
// This is a conservative starting point, not a content-moderation system. A
// product with real exposure should supply its own lists, per language.
package profanity

import (
	"strings"
	"unicode"

	names "github.com/improbabilitea/names-go"
)

// Words are blocked when they appear as a whole selected word.
//
// Matched exactly, so this list can afford to be broad — an entry here can never
// reject a name that does not literally contain that word.
var Words = []string{
	"anal",
	"anus",
	"arse",
	"ass",
	"bastard",
	"bitch",
	"bollocks",
	"boob",
	"boobs",
	"bugger",
	"cock",
	"crap",
	"cunt",
	"dick",
	"dildo",
	"dyke",
	"fag",
	"faggot",
	"fuck",
	"fucker",
	"jizz",
	"nigga",
	"nigger",
	"penis",
	"piss",
	"prick",
	"pube",
	"pussy",
	"queer",
	"rape",
	"rapist",
	"retard",
	"scrotum",
	"semen",
	"shit",
	"slut",
	"spastic",
	"tits",
	"turd",
	"twat",
	"vagina",
	"wank",
	"whore",
}

// Substrings are blocked anywhere in the joined name.
//
// Short and severe by design. Each entry rejects every name containing it,
// including innocent ones, so the bar for adding to this list is that the term
// is offensive enough to be worth losing valid names over. Terms that are
// commonly embedded in ordinary words — "ass" inside "compass", "anal" inside
// "analysis" — are deliberately absent: they belong in Words, where they can
// only match on their own.
var Substrings = []string{
	"cunt",
	"fuck",
	"nigger",
	"nigga",
	"faggot",
	"whore",
	"rapist",
}

// NewWordFilter blocks a name when any of its selected words is in the list.
//
// Runs before formatting, on the individual words, so it sees exactly what the
// vocabulary contributed.
func NewWordFilter(words []string) names.TokenFilter {
	return wordFilter{blocked: normalizeSet(words)}
}

// NewSubstringFilter blocks a name containing any of the terms.
//
// Runs on the formatted name with case and separators stripped, so a term is
// caught however the formatter rendered it — "Fu Cker" and "fu-cker" normalize
// alike.
func NewSubstringFilter(terms []string) names.StringFilter {
	normalized := make([]string, 0, len(terms))
	for _, t := range terms {
		if n := normalize(t); n != "" {
			normalized = append(normalized, n)
		}
	}
	return substringFilter{terms: normalized}
}

type wordFilter struct {
	blocked map[string]struct{}
}

func (wordFilter) Name() string { return "profanity" }

func (f wordFilter) AllowTokens(words []string) bool {
	for _, w := range words {
		if _, blocked := f.blocked[normalize(w)]; blocked {
			return false
		}
	}
	return true
}

type substringFilter struct {
	terms []string
}

func (substringFilter) Name() string { return "profanity" }

func (f substringFilter) Allow(name string) bool {
	normalized := normalize(name)
	for _, term := range f.terms {
		if strings.Contains(normalized, term) {
			return false
		}
	}
	return true
}

func normalizeSet(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		if n := normalize(w); n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}

// normalize reduces a string to lowercase letters and digits, matching what the
// core does for reserved words: separators and punctuation must not be a way
// around the filter.
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
