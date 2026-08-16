// Package weighted implements weighted selection over a fixed set of items.
package weighted

import (
	"sort"

	"github.com/improbabilitea/names-go/internal/random"
)

// Table is a precomputed cumulative-weight table.
//
// Built once at load time and then read-only, which is what allows a Generator
// to be shared across goroutines without synchronization.
//
// Cumulative weights plus binary search, rather than Walker's alias method: the
// alias table is O(1) to sample but its construction order affects its layout,
// which is more surface for non-determinism to creep in, and the gain over
// O(log n) is irrelevant on dictionaries of a few thousand entries.
type Table struct {
	// cum[i] is the sum of weights 0..i, so the interval belonging to item i is
	// [cum[i-1], cum[i]).
	cum   []uint64
	total uint64
}

// New builds a table from per-item weights.
//
// Weights are uint64 rather than the uint32 a manifest declares, because
// template weights reach here already composed: a per-pack weight multiplied by
// a normalization factor, which routinely exceeds the range a single declared
// weight occupies.
func New(weights []uint64) Table {
	cum := make([]uint64, len(weights))

	var running uint64
	for i, w := range weights {
		running += w
		cum[i] = running
	}

	return Table{cum: cum, total: running}
}

// Len returns how many items the table holds.
func (t Table) Len() int { return len(t.cum) }

// CumulativeAt returns the cumulative weight through item i.
//
// Exposed so a caller can recover an individual item's weight — cum[i] minus
// cum[i-1] — which is what building a restricted table over a subset of the
// items requires, and what cardinality analysis needs to weigh each template.
func (t Table) CumulativeAt(i int) uint64 {
	if i < 0 || i >= len(t.cum) {
		return 0
	}
	return t.cum[i]
}

// WeightAt returns item i's own weight.
func (t Table) WeightAt(i int) uint64 {
	if i < 0 || i >= len(t.cum) {
		return 0
	}
	if i == 0 {
		return t.cum[0]
	}
	return t.cum[i] - t.cum[i-1]
}

// Total returns the sum of all weights.
func (t Table) Total() uint64 { return t.total }

// Pick draws an item index from src, or -1 if the table is empty.
//
// Callers validate non-emptiness at load time, so -1 means a bug upstream rather
// than a condition to handle; it is returned instead of panicking because a
// panic on the generation path would turn a configuration mistake into a
// process-level failure.
func (t Table) Pick(src random.Source) int {
	if t.total == 0 {
		return -1
	}
	return t.PickAt(random.Uint64N(src, t.total))
}

// PickAt resolves an already-drawn value in [0, Total) to an item index.
//
// Separate from Pick so tests can assert the interval boundaries directly
// instead of inferring the mapping from a stream of samples.
func (t Table) PickAt(r uint64) int {
	if t.total == 0 {
		return -1
	}
	// The smallest i whose cumulative weight exceeds r — that is the interval r
	// falls into. Items with weight 0 have an empty interval and can never be
	// selected, which is why an explicit zero weight is rejected at load.
	return sort.Search(len(t.cum), func(i int) bool { return t.cum[i] > r })
}
