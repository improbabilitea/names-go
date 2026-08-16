package names

// Result is one generated name.
type Result struct {
	// Value is the formatted name, ready to use.
	Value string

	// Words are the selected tokens before formatting. Keep them if the same
	// logical name has to be re-rendered in another format later — that costs
	// nothing, whereas splitting Value back apart is guesswork.
	Words []string

	// Metadata describes how this name came about.
	Metadata Metadata
}

// Metadata is what the library knows about a generation, exposed as plain data.
//
// The core takes no dependency on any telemetry system, so this is the seam: a
// consumer reads these fields and publishes them wherever it already publishes
// metrics, and the library stays ignorant of OpenTelemetry, CloudWatch and the
// rest.
type Metadata struct {
	// Attempts is how many candidates were built, including the one returned.
	// Anything above 1 means filters rejected something.
	Attempts int

	// Template is the pattern that produced this name.
	Template []string

	// Packs names the packs that contributed the selected words, in the order
	// the words appear.
	Packs []string

	// Step names the escalation rung this name came from — "2-word templates",
	// "3-digit suffix".
	//
	// This is the metric to watch when names must be unique. Traffic moving down
	// the ladder means the short-name space is filling up, and it is the signal
	// that arrives well before users start being handed suffixed names.
	Step string

	// Rejections counts rejections by filter name, and is nil when the first
	// candidate was accepted.
	//
	// Worth publishing as a metric: a rising rejection rate on `length` is the
	// early warning that a vocabulary and a constraint are drifting apart, well
	// before it turns into a user-visible failure.
	Rejections map[string]int

	// Fingerprint identifies the pack set that produced this name. Store it
	// with anything that depends on seeded output.
	Fingerprint string
}
