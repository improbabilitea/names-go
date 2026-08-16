package names

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Format is how the selected words are rendered into a single name.
//
// Formatting is kept apart from selection so one logical name — the token list
// ["midnight", "frame"] — can be rendered as a handle, a URL segment or a label
// without the vocabulary knowing anything about where it will be shown.
type Format uint8

const (
	// Username joins the words directly: "midnightframe".
	Username Format = iota

	// Slug joins the words with hyphens: "midnight-frame".
	Slug

	// DisplayName capitalizes each word and joins with spaces: "Midnight Frame".
	DisplayName

	// Snake joins the words with underscores: "midnight_frame".
	Snake

	// Pascal capitalizes each word and joins them directly: "MidnightFrame".
	Pascal
)

// String returns the format name, as it appears in errors.
func (f Format) String() string {
	switch f {
	case Username:
		return "username"
	case Slug:
		return "slug"
	case DisplayName:
		return "display-name"
	case Snake:
		return "snake"
	case Pascal:
		return "pascal"
	default:
		return "unknown"
	}
}

// valid reports whether f is one of the declared constants.
func (f Format) valid() bool { return f <= Pascal }

// separator returns the string placed between words.
func (f Format) separator() string {
	switch f {
	case Slug:
		return "-"
	case Snake:
		return "_"
	case DisplayName:
		return " "
	default:
		return ""
	}
}

// capitalizes reports whether the format uppercases the first letter of each
// word, which is what makes a lowercase-only charset incompatible with it.
func (f Format) capitalizes() bool {
	return f == DisplayName || f == Pascal
}

// apply renders the words.
//
// Pure and allocation-conscious: one builder sized up front, because this runs
// once per generation attempt and a rejected candidate has already paid for it.
func (f Format) apply(words []string) string {
	if len(words) == 0 {
		return ""
	}

	separator := f.separator()

	size := len(separator) * (len(words) - 1)
	for _, w := range words {
		size += len(w)
	}

	var b strings.Builder
	b.Grow(size)

	for i, word := range words {
		if i > 0 {
			b.WriteString(separator)
		}
		if f.capitalizes() {
			writeCapitalized(&b, word)
		} else {
			b.WriteString(word)
		}
	}

	return b.String()
}

// writeCapitalized writes word with its first rune uppercased.
//
// Only the first rune is touched: the rest of a vocabulary word is already
// lowercase, and forcing the tail would mangle any word a localized pack spells
// with an internal capital.
func writeCapitalized(b *strings.Builder, word string) {
	if word == "" {
		return
	}

	first, size := utf8.DecodeRuneInString(word)
	upper := unicode.ToUpper(first)

	// Uppercasing can change a rune's encoded length (ß, ﬁ and friends), so the
	// tail is appended separately rather than patched in place.
	b.WriteRune(upper)
	b.WriteString(word[size:])
}

// sampleOutput renders a representative name for this format, used at
// construction time to check a charset can express it at all.
func (f Format) sampleOutput() string {
	return f.apply([]string{"alpha", "beta"})
}
