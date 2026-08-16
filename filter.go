package names

import (
	"strings"
	"unicode"
)

// TokenFilter inspects the selected words before they are formatted.
//
// Use it for anything that depends on the *seam* between words — repetition,
// adjacency — since after formatting the boundaries are gone.
type TokenFilter interface {
	// Name identifies the filter in the Rejections breakdown, so keep it short
	// and stable: it ends up in a consumer's metrics.
	Name() string

	// AllowTokens reports whether the candidate may proceed.
	AllowTokens(words []string) bool
}

// StringFilter inspects the formatted name.
//
// Use it for anything about the final string — length, alphabet, blocked terms —
// including the case where two innocuous words become something unwanted only
// once concatenated.
type StringFilter interface {
	Name() string

	// Allow reports whether the candidate may proceed.
	Allow(name string) bool
}

// filterFunc adapts a plain function to StringFilter.
type filterFunc struct {
	name  string
	allow func(string) bool
}

func (f filterFunc) Name() string        { return f.name }
func (f filterFunc) Allow(s string) bool { return f.allow(s) }

// FilterFunc builds a StringFilter from a function, for the common case where a
// custom rule needs no state.
//
//	names.WithFilter(names.FilterFunc("no-digits", func(s string) bool {
//	    return !strings.ContainsAny(s, "0123456789")
//	}))
func FilterFunc(name string, allow func(name string) bool) StringFilter {
	return filterFunc{name: name, allow: allow}
}

// --- built-in filters -------------------------------------------------------

// lengthFilter enforces the configured bounds.
//
// Length is counted in runes, not bytes: a consumer writing MaxLength(20) means
// twenty characters, and a vocabulary with any accented word would otherwise be
// cut short by a limit that silently means something else.
type lengthFilter struct {
	min, max int
}

func (lengthFilter) Name() string { return "length" }

func (f lengthFilter) Allow(name string) bool {
	n := len([]rune(name))
	if f.min > 0 && n < f.min {
		return false
	}
	if f.max > 0 && n > f.max {
		return false
	}
	return true
}

// charsetFilter enforces the configured alphabet.
type charsetFilter struct {
	charset Charset
}

func (charsetFilter) Name() string { return "charset" }

func (f charsetFilter) Allow(name string) bool { return f.charset.allows(name) }

// reservedFilter blocks terms a product needs to keep for itself.
//
// It runs against the individual words *and* against the formatted name with its
// separators removed, because "sup-port" as a slug is still "support" to anyone
// reading it, and a reserved list that only matched exact output would be
// trivially sidestepped by the formatter.
type reservedFilter struct {
	words map[string]struct{}
}

func newReservedFilter(words []string) reservedFilter {
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		if normalized := normalizeForMatching(w); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return reservedFilter{words: set}
}

func (reservedFilter) Name() string { return "reserved" }

func (f reservedFilter) AllowTokens(words []string) bool {
	for _, w := range words {
		if _, blocked := f.words[normalizeForMatching(w)]; blocked {
			return false
		}
	}
	return true
}

func (f reservedFilter) Allow(name string) bool {
	_, blocked := f.words[normalizeForMatching(name)]
	return !blocked
}

// normalizeForMatching reduces a string to what a reader would perceive:
// lowercase letters and digits, with separators and punctuation dropped.
func normalizeForMatching(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// repetitionFilter rejects candidates whose words collide with each other.
//
// Weighted selection is free to draw "silent" twice, or to put "golden" next to
// "denim" — each choice is individually fine and the pair is not. Three
// situations are caught:
//
//   - the same word twice, giving "silentsilent";
//   - one word contained at the edge of its neighbour, giving "silentsilently";
//   - a shared seam, where the tail of one word is the head of the next:
//     "golden" + "denim" reads as "goldendenim", and "silent" + "take" doubles
//     the t.
//
// The seam rule is the one that earns its place. The first two are rare; the
// third fires on roughly one pair in twenty, and those are precisely the names a
// human would look at and call sloppy.
type repetitionFilter struct{}

func (repetitionFilter) Name() string { return "repetition" }

func (repetitionFilter) AllowTokens(words []string) bool {
	for i := 1; i < len(words); i++ {
		previous, current := words[i-1], words[i]

		if previous == current {
			return false
		}
		if strings.HasPrefix(current, previous) || strings.HasSuffix(current, previous) ||
			strings.HasPrefix(previous, current) || strings.HasSuffix(previous, current) {
			return false
		}
		if sharesSeam(previous, current) {
			return false
		}
	}

	// Non-adjacent repetition still reads as a mistake in a three-word name.
	for i := range words {
		for j := i + 2; j < len(words); j++ {
			if words[i] == words[j] {
				return false
			}
		}
	}

	return true
}

// sharesSeam reports whether the tail of a is the head of b.
//
// Checked over runes rather than bytes so a multi-byte character cannot be split
// mid-encoding, which would compare meaningless fragments.
func sharesSeam(a, b string) bool {
	left, right := []rune(a), []rune(b)

	limit := min(len(left), len(right))
	for k := 1; k <= limit; k++ {
		if string(left[len(left)-k:]) == string(right[:k]) {
			return true
		}
	}
	return false
}

// --- pipeline ---------------------------------------------------------------

// pipeline holds the filters in the order they run.
//
// Cheapest and most-rejecting first, so the expensive checks see the fewest
// candidates. Token filters necessarily precede formatting; string filters
// necessarily follow it.
type pipeline struct {
	token  []TokenFilter
	string []StringFilter
}

// allowTokens runs the pre-format stage, returning the name of the filter that
// rejected the candidate, or "" if it passed.
func (p pipeline) allowTokens(words []string) string {
	for _, f := range p.token {
		if !f.AllowTokens(words) {
			return f.Name()
		}
	}
	return ""
}

// allowString runs the post-format stage, returning the rejecting filter's name,
// or "" if the candidate passed.
func (p pipeline) allowString(name string) string {
	for _, f := range p.string {
		if !f.Allow(name) {
			return f.Name()
		}
	}
	return ""
}
