package random

import (
	"math"
	"testing"
)

// scriptedSource replays a fixed sequence, so tests can drive the rejection
// branch of Uint64N instead of hoping a real stream wanders into it.
type scriptedSource struct {
	values []uint64
	i      int
}

func (s *scriptedSource) Uint64() uint64 {
	if s.i >= len(s.values) {
		panic("scriptedSource: exhausted")
	}
	v := s.values[s.i]
	s.i++
	return v
}

func testSource(t *testing.T) Source {
	t.Helper()
	return Derive("test", []byte("fingerprint"), "source")
}

// TestDerive_GoldenVectors pins the derived stream to known values.
//
// This is the architecture- and toolchain-stability check: SHA-256 and PCG are
// both specified algorithms, so these numbers must not move on a different CPU
// or a newer Go. If they do, the determinism contract has broken and every
// seeded name in existence has changed with it.
func TestDerive_GoldenVectors(t *testing.T) {
	cases := []struct {
		name    string
		version string
		finger  string
		seed    string
		want    [4]uint64
	}{
		{
			name:    "baseline",
			version: "v1",
			finger:  "fingerprint",
			seed:    "user_01893",
			want: [4]uint64{
				0x81290b1c7194feed, 0xe204d5f8aec8c1ab,
				0xc5250764e004f7f6, 0x7e00b7737b5ee3b3,
			},
		},
		{
			name:    "empty seed",
			version: "v1",
			finger:  "fingerprint",
			seed:    "",
			want: [4]uint64{
				0x3b6d78dad78faeef, 0xc234324bb7b819f1,
				0x19c86340d85213ed, 0x2afdddbb613173ae,
			},
		},
		{
			name:    "algorithm version changes the stream",
			version: "v2",
			finger:  "fingerprint",
			seed:    "user_01893",
			want: [4]uint64{
				0x90dfe2dfead4bdf0, 0xdb62243e583e9e77,
				0x9c56b3936c705a6c, 0x6fe4ae694c1ad46b,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := Derive(tc.version, []byte(tc.finger), tc.seed)
			for i, want := range tc.want {
				if got := src.Uint64(); got != want {
					t.Fatalf("value %d = 0x%016x, want 0x%016x", i, got, want)
				}
			}
		})
	}
}

// TestDerive_LengthPrefixDisambiguates is the reason writeField prefixes each
// component with its length. Without it these two calls concatenate to the same
// bytes and share a stream, so two unrelated configurations would silently
// generate identical names.
func TestDerive_LengthPrefixDisambiguates(t *testing.T) {
	a := Derive("ab", []byte("c"), "")
	b := Derive("a", []byte("bc"), "")

	if a.Uint64() == b.Uint64() {
		t.Fatal(`Derive("ab","c") and Derive("a","bc") share a stream; the length prefix is not doing its job`)
	}
}

func TestDerive_IsDeterministic(t *testing.T) {
	a := Derive("v1", []byte("fp"), "seed")
	b := Derive("v1", []byte("fp"), "seed")

	for i := range 32 {
		x, y := a.Uint64(), b.Uint64()
		if x != y {
			t.Fatalf("value %d diverged: 0x%016x != 0x%016x", i, x, y)
		}
	}
}

func TestDerive_FingerprintChangesStream(t *testing.T) {
	a := Derive("v1", []byte("fp-one"), "seed")
	b := Derive("v1", []byte("fp-two"), "seed")

	if a.Uint64() == b.Uint64() {
		t.Fatal("a different pack fingerprint produced the same stream")
	}
}

// TestDerive_ConsecutiveSeedsAreUncorrelated covers the case the hashing exists
// for: sequential user ids must not produce neighbouring streams, or names
// would cluster for users who registered one after the other.
func TestDerive_ConsecutiveSeedsAreUncorrelated(t *testing.T) {
	seen := make(map[uint64]string)

	for _, seed := range []string{
		"user_1", "user_2", "user_3", "user_4", "user_5",
		"user_6", "user_7", "user_8", "user_9",
	} {
		v := Derive("v1", []byte("fp"), seed).Uint64()
		if other, dup := seen[v]; dup {
			t.Fatalf("seeds %q and %q produced the same first value 0x%016x", other, seed, v)
		}
		seen[v] = seed
	}
}

func TestUint64N_PanicsOnZero(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Uint64N(src, 0) did not panic")
		}
	}()
	_ = Uint64N(testSource(t), 0)
}

func TestUint64N_BoundOfOneIsAlwaysZero(t *testing.T) {
	src := testSource(t)
	for range 100 {
		if got := Uint64N(src, 1); got != 0 {
			t.Fatalf("Uint64N(src, 1) = %d, want 0", got)
		}
	}
}

func TestUint64N_StaysInRange(t *testing.T) {
	src := testSource(t)
	for _, n := range []uint64{2, 3, 7, 16, 100, 1 << 32, math.MaxUint64} {
		for range 1000 {
			if got := Uint64N(src, n); got >= n {
				t.Fatalf("Uint64N(src, %d) = %d, out of range", n, got)
			}
		}
	}
}

// TestUint64N_PowerOfTwoNeverRejects checks the threshold arithmetic: for a
// power of two, 2^64 mod n is 0, so every draw must be consumed. A source with
// exactly one value available proves no second draw happened.
func TestUint64N_PowerOfTwoNeverRejects(t *testing.T) {
	src := &scriptedSource{values: []uint64{0}}
	if got := Uint64N(src, 8); got != 0 {
		t.Fatalf("Uint64N = %d, want 0", got)
	}
}

// TestUint64N_RejectsBiasedTail pins the rejection branch. For n = 3 the tail is
// 2^64 mod 3 = 1, so the single value 0 must be discarded and the next one used.
// Without rejection this would return 0 instead of 2, and low residues would be
// over-represented — which in practice means the alphabetically first words in
// every dictionary get picked slightly too often.
func TestUint64N_RejectsBiasedTail(t *testing.T) {
	src := &scriptedSource{values: []uint64{0, 5}}

	got := Uint64N(src, 3)
	if got != 2 {
		t.Fatalf("Uint64N = %d, want 2 (5 %% 3)", got)
	}
	if src.i != 2 {
		t.Fatalf("consumed %d values, want 2 (the biased tail value must be rejected)", src.i)
	}
}

func TestUint64N_IsUniform(t *testing.T) {
	const (
		draws   = 1_000_000
		buckets = 7 // not a power of two, so the rejection path is exercised
	)

	src := testSource(t)
	counts := make([]int, buckets)
	for range draws {
		counts[Uint64N(src, buckets)]++
	}

	expected := float64(draws) / buckets
	for i, got := range counts {
		if deviation := math.Abs(float64(got)-expected) / expected; deviation > 0.01 {
			t.Errorf("bucket %d: %d draws, expected ~%.0f (deviation %.3f%%)",
				i, got, expected, deviation*100)
		}
	}
}

func TestNewPCG_ProducesDistinctStreams(t *testing.T) {
	first, second := NewPCG(), NewPCG()
	if a, b := first.Uint64(), second.Uint64(); a == b {
		t.Fatalf("two unseeded sources produced the same first value 0x%016x", a)
	}
}

func TestNewCrypto_ProducesDistinctStreams(t *testing.T) {
	first, second := NewCrypto(), NewCrypto()
	if a, b := first.Uint64(), second.Uint64(); a == b {
		t.Fatalf("two crypto sources produced the same first value 0x%016x", a)
	}
}
