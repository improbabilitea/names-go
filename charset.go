package names

import "strings"

// asciiRange is how many code points a Charset tracks directly.
const asciiRange = 128

// Charset is the set of characters an output name may contain.
//
// The zero value imposes no restriction, which is the right default for a
// display name — free text where accented and non-Latin characters are a feature
// rather than a problem. Products that use names as handles or path segments opt
// into a narrower set.
type Charset struct {
	// allowed is nil when the charset is unset, which is what makes the zero
	// value permissive rather than "allows nothing".
	allowed *[asciiRange]bool

	label string
}

// Predefined charsets covering the cases a consumer actually configures.
//
// Each is paired with the formats it can express: CharsetLowerAlnum cannot
// produce a slug, because a slug contains hyphens. New rejects that combination
// at construction rather than letting every candidate fail a filter at runtime.
var (
	// CharsetLowerAlpha allows a-z only.
	CharsetLowerAlpha = CharsetOf("lower-alpha", "abcdefghijklmnopqrstuvwxyz")

	// CharsetLowerAlnum allows a-z and 0-9. Suits the Username format.
	CharsetLowerAlnum = CharsetOf("lower-alnum", "abcdefghijklmnopqrstuvwxyz0123456789")

	// CharsetSlug allows a-z, 0-9 and the hyphen. Suits the Slug format.
	CharsetSlug = CharsetOf("slug", "abcdefghijklmnopqrstuvwxyz0123456789-")

	// CharsetSnake allows a-z, 0-9 and the underscore. Suits the Snake format.
	CharsetSnake = CharsetOf("snake", "abcdefghijklmnopqrstuvwxyz0123456789_")
)

// CharsetOf builds a charset from the characters it permits.
//
// The label is what a rejection is reported under, so give it something a
// developer reading a Rejections map will recognise.
//
// Only ASCII characters can be permitted: a charset exists to constrain output
// to a mechanically safe alphabet, and enumerating a permitted subset of Unicode
// is not that job. A name containing any non-ASCII rune fails a set charset.
func CharsetOf(label, chars string) Charset {
	var allowed [asciiRange]bool
	for _, r := range chars {
		if r < asciiRange {
			allowed[r] = true
		}
	}
	return Charset{allowed: &allowed, label: label}
}

// isSet reports whether this charset constrains anything.
func (c Charset) isSet() bool { return c.allowed != nil }

// allowsRune reports whether r is permitted.
func (c Charset) allowsRune(r rune) bool {
	if !c.isSet() {
		return true
	}
	if r < 0 || r >= asciiRange {
		return false
	}
	return c.allowed[r]
}

// allows reports whether every character of s is permitted.
func (c Charset) allows(s string) bool {
	if !c.isSet() {
		return true
	}
	for _, r := range s {
		if !c.allowsRune(r) {
			return false
		}
	}
	return true
}

// missing returns the characters of s this charset rejects, deduplicated and in
// order of first appearance. Used to explain a construction-time contradiction.
func (c Charset) missing(s string) string {
	if !c.isSet() {
		return ""
	}

	var b strings.Builder
	seen := make(map[rune]bool)
	for _, r := range s {
		if c.allowsRune(r) || seen[r] {
			continue
		}
		seen[r] = true
		b.WriteRune(r)
	}
	return b.String()
}

// String returns the charset label, or "unrestricted" for the zero value.
func (c Charset) String() string {
	if !c.isSet() {
		return "unrestricted"
	}
	return c.label
}
