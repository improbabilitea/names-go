package names

// Pack is a coherent set of dictionaries and templates.
//
// It is deliberately opaque: a Pack only ever comes out of LoadFS, which is
// what guarantees every Pack in existence has been validated and canonically
// ordered. Letting callers assemble one field by field would put unvalidated,
// unsorted packs into the engine and quietly break the determinism contract.
type Pack struct {
	name    string
	version string

	// dictionaries is ordered by category; each holds words in canonical order.
	dictionaries []Dictionary

	// templates is in canonical order.
	templates []Template
}

// registeredPack pairs a pack with the weight the consumer gave it at
// registration.
//
// The weight lives here rather than on Pack because it is not a property of the
// vocabulary: the same cinema pack is a flourish in one product and the dominant
// voice in another, and only the consumer knows which.
type registeredPack struct {
	pack   *Pack
	weight uint32
}

// Name returns the pack name declared in its manifest.
func (p *Pack) Name() string { return p.name }

// Version returns the pack version declared in its manifest, or "" if the
// manifest omitted it.
//
// The library does not interpret this value. It exists so a pack author can
// record what a consumer would otherwise have to infer from the fingerprint.
func (p *Pack) Version() string { return p.version }

// Categories returns the category names this pack provides, in canonical order.
//
// Useful to a consumer diagnosing an ErrDictionaryNotFound, which is otherwise
// a guessing game about which pack was supposed to supply the missing category.
func (p *Pack) Categories() []string {
	out := make([]string, 0, len(p.dictionaries))
	for _, d := range p.dictionaries {
		out = append(out, d.Category)
	}
	return out
}
