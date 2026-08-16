package weighted

import (
	"math"
	"testing"

	"github.com/improbabilitea/names-go/internal/random"
)

func testSource(t *testing.T) random.Source {
	t.Helper()
	return random.Derive("test", []byte("fingerprint"), "weighted")
}

func TestNew_TotalIsTheSumOfWeights(t *testing.T) {
	table := New([]uint64{1, 2, 3, 4})

	if got := table.Total(); got != 10 {
		t.Fatalf("Total() = %d, want 10", got)
	}
	if got := table.Len(); got != 4 {
		t.Fatalf("Len() = %d, want 4", got)
	}
}

// TestPickAt_MapsIntervalsExactly asserts the boundary mapping directly rather
// than inferring it from samples: item i owns [cum[i-1], cum[i]), and an
// off-by-one here would skew every selection in the library by one item.
func TestPickAt_MapsIntervalsExactly(t *testing.T) {
	// Weights 1, 2, 3 -> intervals [0,1), [1,3), [3,6).
	table := New([]uint64{1, 2, 3})

	want := []int{0, 1, 1, 2, 2, 2}
	for r, expected := range want {
		if got := table.PickAt(uint64(r)); got != expected {
			t.Errorf("PickAt(%d) = %d, want %d", r, got, expected)
		}
	}
}

func TestPickAt_SingleItem(t *testing.T) {
	table := New([]uint64{5})

	for r := range uint64(5) {
		if got := table.PickAt(r); got != 0 {
			t.Fatalf("PickAt(%d) = %d, want 0", r, got)
		}
	}
}

func TestPick_EmptyTable(t *testing.T) {
	table := New(nil)

	if got := table.Pick(testSource(t)); got != -1 {
		t.Fatalf("Pick on empty table = %d, want -1", got)
	}
	if got := table.PickAt(0); got != -1 {
		t.Fatalf("PickAt on empty table = %d, want -1", got)
	}
}

// TestPickAt_ZeroWeightIsUnreachable documents why an explicit zero weight is a
// load-time error: it produces an empty interval, so the word silently never
// appears. Better to reject the manifest than to ship a vocabulary with dead
// entries in it.
func TestPickAt_ZeroWeightIsUnreachable(t *testing.T) {
	// Weights 1, 0, 1 -> intervals [0,1), empty, [1,2).
	table := New([]uint64{1, 0, 1})

	for r := range table.Total() {
		if got := table.PickAt(r); got == 1 {
			t.Fatalf("PickAt(%d) selected the zero-weight item", r)
		}
	}
}

// TestNew_AccumulatesInUint64 covers the overflow guard. Four items at the top
// of the uint32 range sum past 2^32, and if the accumulator were uint32 the
// cumulative array would wrap and stop being monotonic — which would make the
// binary search return arbitrary items rather than fail loudly.
func TestNew_AccumulatesInUint64(t *testing.T) {
	const w = uint64(math.MaxUint32)
	table := New([]uint64{w, w, w, w})

	if got, want := table.Total(), w*4; got != want {
		t.Fatalf("Total() = %d, want %d", got, want)
	}

	var previous uint64
	for i := range table.Len() {
		if table.cum[i] <= previous {
			t.Fatalf("cumulative weights are not monotonic at %d: %d after %d",
				i, table.cum[i], previous)
		}
		previous = table.cum[i]
	}
}

func TestPick_RespectsWeights(t *testing.T) {
	const draws = 1_000_000

	weights := []uint64{1, 2, 3, 4}
	table := New(weights)
	src := testSource(t)

	counts := make([]int, len(weights))
	for range draws {
		counts[table.Pick(src)]++
	}

	for i, w := range weights {
		expected := float64(draws) * float64(w) / float64(table.Total())
		if deviation := math.Abs(float64(counts[i])-expected) / expected; deviation > 0.01 {
			t.Errorf("item %d (weight %d): %d draws, expected ~%.0f (deviation %.3f%%)",
				i, w, counts[i], expected, deviation*100)
		}
	}
}

func TestPick_EqualWeightsAreUniform(t *testing.T) {
	const (
		draws = 1_000_000
		items = 5
	)

	table := New([]uint64{1, 1, 1, 1, 1})
	src := testSource(t)

	counts := make([]int, items)
	for range draws {
		counts[table.Pick(src)]++
	}

	expected := float64(draws) / items
	for i, got := range counts {
		if deviation := math.Abs(float64(got)-expected) / expected; deviation > 0.01 {
			t.Errorf("item %d: %d draws, expected ~%.0f (deviation %.3f%%)",
				i, got, expected, deviation*100)
		}
	}
}

// TestPick_DominantWeight covers the lopsided case a pack author creates when
// making one word deliberately rare.
func TestPick_DominantWeight(t *testing.T) {
	const draws = 100_000

	table := New([]uint64{999, 1})
	src := testSource(t)

	var rare int
	for range draws {
		if table.Pick(src) == 1 {
			rare++
		}
	}

	expected := float64(draws) / 1000
	if deviation := math.Abs(float64(rare)-expected) / expected; deviation > 0.15 {
		t.Errorf("rare item drawn %d times, expected ~%.0f", rare, expected)
	}
}

func TestPick_IsDeterministicForTheSameStream(t *testing.T) {
	table := New([]uint64{3, 1, 4, 1, 5})

	first := make([]int, 100)
	src := testSource(t)
	for i := range first {
		first[i] = table.Pick(src)
	}

	src = testSource(t)
	for i, want := range first {
		if got := table.Pick(src); got != want {
			t.Fatalf("draw %d = %d, want %d", i, got, want)
		}
	}
}
