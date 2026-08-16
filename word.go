package names

// defaultWeight is what an absent weight means. An explicitly declared zero is
// a validation error instead: silently treating it as "never pick this" would
// hide a typo, and Enabled already expresses that intent.
const defaultWeight uint32 = 1

// Word is one selectable element of a vocabulary.
//
// A word is not just a string because the vocabulary has to carry the
// information the engine selects on — which category it belongs to, how often it
// should come up — and the information a future version will select on, which is
// what Tags and Locale are reserved for.
type Word struct {
	// Value is the textual content, lowercase and unformatted. Formatting is
	// applied later, by the formatter, so the same word can appear as "frame",
	// "Frame" or "-frame" without being stored three times.
	Value string

	// Category is the dictionary this word belongs to, and the name a template
	// references. In v1 categories are flat strings, so a theme pack namespaces
	// its own by convention (`cinema_noun`).
	Category string

	// Tags are optional classifications. They are read, validated and preserved
	// but do not take part in selection in v1: they exist so that templates can
	// later select by category *and* tag without a data migration.
	Tags []string

	// Weight is the relative frequency within its category. Absent means 1;
	// zero is rejected at load.
	Weight uint32

	// Locale is an optional language marker. Like Tags, it is carried but not
	// used for selection in v1, where a pack is implicitly single-language.
	Locale string

	// Enabled reports whether the word may be selected.
	//
	// Note the zero value is false. Words are built by the loader, which
	// defaults an absent `enabled` to true; a Word assembled by hand must set
	// this explicitly.
	Enabled bool
}

// weight returns the effective weight, substituting the default for an unset
// one. Validation rejects an explicit zero before this is ever reached, so this
// only covers words built programmatically.
func (w Word) weight() uint32 {
	if w.Weight == 0 {
		return defaultWeight
	}
	return w.Weight
}
