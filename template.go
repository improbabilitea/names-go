package names

import "slices"

// Template is the shape of a name: an ordered list of categories to draw from.
type Template struct {
	// Pattern lists the categories, in output order. `["adjective", "animal"]`
	// produces names like "velvetfox".
	Pattern []string

	// Weight is the relative frequency of this pattern *within its own pack*.
	// Absent means 1; zero is rejected at load.
	//
	// Weights are not comparable across packs: they are normalized per pack and
	// then composed with the pack weight, so a pack author controls the mix
	// inside their theme and the consumer controls the mix between themes.
	Weight uint32
}

// Arity is how many words the template draws.
//
// It is a selection attribute rather than a derived detail: the escalation
// ladder picks templates by arity when it widens a name from two words to
// three.
func (t Template) Arity() int { return len(t.Pattern) }

// weight returns the effective weight, substituting the default for an unset one.
func (t Template) weight() uint32 {
	if t.Weight == 0 {
		return defaultWeight
	}
	return t.Weight
}

// sortTemplatesCanonical orders templates by content, for the same reason
// sortWordsCanonical does: reordering the `templates` array in a manifest is a
// cosmetic edit and must not move the fingerprint.
func sortTemplatesCanonical(templates []Template) {
	slices.SortFunc(templates, func(a, b Template) int {
		if c := slices.Compare(a.Pattern, b.Pattern); c != 0 {
			return c
		}
		return compareUint32(a.weight(), b.weight())
	})
}
