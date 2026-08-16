package names

import "slices"

// Dictionary is the set of words in one category.
type Dictionary struct {
	// Category names the dictionary and is what a template's pattern refers to.
	Category string

	// Words is held in canonical order — see sortWordsCanonical.
	Words []Word
}

// sortWordsCanonical puts words in the order the rest of the library assumes.
//
// The order has to be a function of the *content*, never of how the content
// happened to be written down: reordering the lines of a .txt must not change
// the pack fingerprint, and therefore must not change seeded output. Sorting by
// value on load is what makes that true.
//
// The tiebreakers on category and weight only matter for entries that are
// otherwise identical; they exist so the sort is total and the result cannot
// depend on the sort algorithm's handling of equal elements.
func sortWordsCanonical(words []Word) {
	slices.SortFunc(words, func(a, b Word) int {
		if c := compareString(a.Value, b.Value); c != 0 {
			return c
		}
		if c := compareString(a.Category, b.Category); c != 0 {
			return c
		}
		return compareUint32(a.weight(), b.weight())
	})
}

// enabledWords returns the selectable words, preserving order.
func (d Dictionary) enabledWords() []Word {
	out := make([]Word, 0, len(d.Words))
	for _, w := range d.Words {
		if w.Enabled {
			out = append(out, w)
		}
	}
	return out
}

// compareString orders strings bytewise, which is what makes the canonical
// order independent of locale and collation settings.
func compareString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareUint32(a, b uint32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
