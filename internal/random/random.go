// Package random owns every source of randomness in the library, and every
// reduction of a random value into a range.
//
// # Why the reduction lives here
//
// math/rand/v2's PCG produces a specified, stable stream: the same two seed
// words yield the same uint64 sequence, and that is a documented property we can
// build a contract on. The *reduction* methods on rand.Rand — Uint64N, IntN —
// are not that. They are implementation detail, and they have changed before
// (math/rand to math/rand/v2 changed exactly this).
//
// Seeded output is contractually stable for a given algorithm version and pack
// fingerprint. If a reduction algorithm inside the standard library moved, that
// contract would break with nothing in this repository having changed, and the
// golden tests would fail on a toolchain bump. So this package consumes only the
// raw Uint64 stream and does its own reduction, which puts determinism entirely
// under our control.
package random

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	mrand "math/rand/v2"
)

// Source produces uniformly distributed 64-bit values.
//
// Only Uint64 is required, deliberately: see the package comment.
type Source interface {
	Uint64() uint64
}

// Derive builds the deterministic source for a seed.
//
// The caller's seed is never used as generator state directly. It is hashed
// together with the algorithm version and the pack fingerprint, which buys three
// things at once:
//
//   - a seed carrying personal data (an email, an internal id) cannot leak into
//     the output or be recovered from it;
//   - a change of algorithm or of vocabulary automatically yields a different
//     stream, so the versioning requirement needs no separate machinery;
//   - consecutive seeds ("user_1", "user_2") produce uncorrelated streams,
//     which a raw seed would not.
func Derive(algorithmVersion string, fingerprint []byte, seed string) Source {
	h := sha256.New()
	writeField(h, []byte(algorithmVersion))
	writeField(h, fingerprint)
	writeField(h, []byte(seed))
	sum := h.Sum(nil)

	return mrand.NewPCG(
		binary.BigEndian.Uint64(sum[0:8]),
		binary.BigEndian.Uint64(sum[8:16]),
	)
}

// writeField feeds one length-prefixed component into the hash.
//
// The length prefix is what makes the concatenation unambiguous: without it
// ("ab", "c") and ("a", "bc") would hash identically, and two different
// (version, fingerprint, seed) triples would silently share a stream.
func writeField(h hash.Hash, b []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(b)))
	// hash.Hash documents that Write never returns an error.
	_, _ = h.Write(length[:])
	_, _ = h.Write(b)
}

// Uint64N returns a uniformly distributed value in [0, n). It panics if n is 0.
//
// The bias that naive `Uint64() % n` introduces is real: unless n divides 2^64,
// the low residues get one extra representative each, which skews selection
// toward whichever words sort first. Rejection sampling removes it — discard the
// leading 2^64 mod n values, and what remains divides evenly.
func Uint64N(src Source, n uint64) uint64 {
	if n == 0 {
		panic("names/internal/random: Uint64N with n == 0")
	}

	// -n % n is 2^64 mod n in wrapping unsigned arithmetic: the size of the
	// unusable tail. For a power of two it is 0 and nothing is ever rejected.
	threshold := -n % n
	for {
		u := src.Uint64()
		if u >= threshold {
			return u % n
		}
	}
}

// NewPCG returns a PCG source seeded from the global source.
//
// This is the default for unseeded generation. The global rand/v2 source is
// goroutine-safe and implemented per-P, so seeding from it costs no contention,
// and the returned PCG is local to one call — which is what lets a Generator be
// shared across goroutines without a lock.
func NewPCG() Source {
	return mrand.NewPCG(mrand.Uint64(), mrand.Uint64())
}

// NewCrypto returns a crypto/rand-backed source.
//
// Opt-in, for consumers whose names act as public identifiers and therefore must
// not be predictable. PCG is fast and reproducible but explicitly unsuitable
// there: given a little output it can be reconstructed.
func NewCrypto() Source { return cryptoSource{} }

type cryptoSource struct{}

func (cryptoSource) Uint64() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read is documented never to fail on any supported
		// platform. If that ever stops holding, returning a predictable value
		// would defeat the only reason to have chosen this source.
		panic("names/internal/random: crypto/rand unavailable: " + err.Error())
	}
	return binary.BigEndian.Uint64(b[:])
}
