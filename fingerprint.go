package names

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

// algorithmVersion identifies the selection algorithm itself.
//
// It is hashed into every seeded stream, so bumping it re-rolls every seeded
// name in existence. Bump it when the *way* a name is chosen changes — the order
// of random draws, the weighting arithmetic — not when a vocabulary changes,
// which the pack fingerprint already covers.
const algorithmVersion = "v1"

// fingerprint identifies a merged pack set by its content.
type fingerprint struct {
	sum [32]byte
}

// String renders the fingerprint for logs, metadata and golden files.
func (f fingerprint) String() string {
	return "sha256:" + hex.EncodeToString(f.sum[:])
}

// bytes returns the raw digest, which is what feeds seed derivation.
func (f fingerprint) bytes() []byte { return f.sum[:] }

// computeFingerprint hashes the canonical form of an ordered set of packs.
//
// # What goes in, and what deliberately does not
//
// Everything that can change the output goes in: pack names, registration order,
// pack weights, categories, the enabled words with their weights, and the
// templates with their patterns and weights.
//
// Nothing cosmetic goes in. Line order inside a .txt, file names, comments, JSON
// indentation, the pack's declared version — none of it reaches the fingerprint,
// because none of it reaches the output. Reordering a vocabulary file is a
// no-op, and that is a property worth having: curating a word list should not
// silently re-roll every seeded name.
//
// Tags and locale are excluded on the same grounds. They are carried on Word but
// take no part in v1 selection, so enriching a vocabulary's metadata leaves
// seeded output untouched. When a future version starts selecting on them, they
// join the fingerprint and that release re-rolls seeded names — which is exactly
// what the contract in the README says can happen across pack versions.
//
// Disabled words are skipped entirely rather than hashed with a flag: a word
// that cannot be selected is, as far as the output is concerned, not there.
func computeFingerprint(packs []registeredPack) fingerprint {
	h := sha256.New()

	// Every variable-length run is preceded by its count. Without that, two
	// packs of one word each and one pack of two words could serialize
	// identically, and two different configurations would share a stream.
	writeCount(h, len(packs))

	for _, registered := range packs {
		pack := registered.pack

		writeField(h, pack.name)
		writeUint(h, uint64(registered.weight))

		writeCount(h, len(pack.dictionaries))
		for _, dictionary := range pack.dictionaries {
			writeField(h, dictionary.Category)

			enabled := dictionary.enabledWords()
			writeCount(h, len(enabled))
			for _, word := range enabled {
				writeField(h, word.Value)
				writeUint(h, uint64(word.weight()))
			}
		}

		writeCount(h, len(pack.templates))
		for _, template := range pack.templates {
			writeCount(h, len(template.Pattern))
			for _, category := range template.Pattern {
				writeField(h, category)
			}
			writeUint(h, uint64(template.weight()))
		}
	}

	var f fingerprint
	h.Sum(f.sum[:0])
	return f
}

// writeField feeds one length-prefixed string into the hash, so that
// ("ab", "c") and ("a", "bc") cannot hash alike.
func writeField(h hash.Hash, s string) {
	writeUint(h, uint64(len(s)))
	// hash.Hash documents that Write never returns an error.
	_, _ = h.Write([]byte(s))
}

// writeCount prefixes a variable-length run with its element count.
//
// n is always a len(), so it cannot be negative and the conversion cannot wrap.
func writeCount(h hash.Hash, n int) {
	writeUint(h, uint64(n)) //#nosec G115 -- n is a slice length
}

func writeUint(h hash.Hash, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, _ = h.Write(b[:])
}
