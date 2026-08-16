package names

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The sentinels below are the stable classification surface: callers match with
// errors.Is and never on message text.
//
// Two of them — ErrInvalidPack and friends, and ErrNoValidCandidate — are also
// carried by structured types further down, because a bare sentinel cannot say
// *which* pack was wrong or *why* generation gave up. The sentinel stays the
// thing you compare against; the type is what you inspect once you know.
var (
	// ErrNoPack is returned by New when no pack was registered. A generator
	// without a vocabulary has nothing to select from.
	ErrNoPack = errors.New("names: no pack registered")

	// ErrNoTemplate is returned by New when the merged pack set declares no
	// template, so no name shape exists.
	ErrNoTemplate = errors.New("names: no template available")

	// ErrDictionaryNotFound is returned by New when a template references a
	// category that no registered pack provides. This is the error a theme pack
	// loaded without its base pack produces.
	ErrDictionaryNotFound = errors.New("names: dictionary not found")

	// ErrInvalidPack is returned by LoadFS when a pack cannot be read or is
	// structurally unsound.
	ErrInvalidPack = errors.New("names: invalid pack")

	// ErrInvalidTemplate is returned when a template's pattern is empty or
	// otherwise unusable.
	ErrInvalidTemplate = errors.New("names: invalid template")

	// ErrInvalidWeight is returned when a weight is explicitly zero. Absent
	// weights default to 1; zero is a mistake, and disabling a word is what
	// Word.Enabled is for.
	ErrInvalidWeight = errors.New("names: invalid weight")

	// ErrEmptyDictionary is returned when a declared category contains no
	// usable word.
	ErrEmptyDictionary = errors.New("names: empty dictionary")

	// ErrNoValidCandidate is returned by Generate when every attempt was
	// rejected. See NoValidCandidateError for the breakdown.
	ErrNoValidCandidate = errors.New("names: no valid candidate")

	// ErrInvalidOptions is returned by New when the options contradict each
	// other, for example a minimum length above the maximum.
	ErrInvalidOptions = errors.New("names: invalid options")

	// ErrUnsupportedFormat is returned when a Format value is not one of the
	// declared constants.
	ErrUnsupportedFormat = errors.New("names: unsupported format")
)

// PackValidationError says which pack failed validation, on which field, and
// why.
//
// Configuration errors surface at load time, not on a user request, so this is
// what a developer sees when a pack is wrong — it has to be actionable without
// reading the library's source.
type PackValidationError struct {
	// Pack is the pack name, or the manifest path when the failure happened
	// before a name could be read.
	Pack string

	// Field locates the problem inside the manifest, e.g. `templates[2].pattern`
	// or `dictionaries.cinema_noun`.
	Field string

	// Reason explains the failure in one clause.
	Reason string

	sentinel error
}

func (e *PackValidationError) Error() string {
	var b strings.Builder
	b.WriteString("names: pack ")
	b.WriteString(strconv.Quote(e.Pack))
	if e.Field != "" {
		b.WriteString(": ")
		b.WriteString(e.Field)
	}
	b.WriteString(": ")
	b.WriteString(e.Reason)
	return b.String()
}

// Unwrap returns the sentinel this failure classifies as, so errors.Is keeps
// working as the primary idiom.
func (e *PackValidationError) Unwrap() error { return e.sentinel }

// packErrorf builds a PackValidationError classified under sentinel.
func packErrorf(sentinel error, pack, field, format string, args ...any) *PackValidationError {
	return &PackValidationError{
		Pack:     pack,
		Field:    field,
		Reason:   fmt.Sprintf(format, args...),
		sentinel: sentinel,
	}
}

// NoValidCandidateError reports that generation exhausted its attempts, and
// which filters did the rejecting.
//
// This is the failure a consumer actually meets in production, and without the
// per-filter breakdown it is undiagnosable: "no valid candidate" does not
// distinguish a vocabulary that outgrew its length constraint from a reserved
// list that is too broad.
type NoValidCandidateError struct {
	// Attempts is how many candidates were built and rejected.
	Attempts int

	// Rejections counts rejections per filter name. A candidate is counted
	// against the first filter that rejected it, so the counts sum to Attempts.
	Rejections map[string]int
}

func (e *NoValidCandidateError) Error() string {
	if len(e.Rejections) == 0 {
		return fmt.Sprintf("names: no valid candidate after %d attempts", e.Attempts)
	}

	// Sorted: a map range would make the message vary between identical runs,
	// which is exactly the property this library sells.
	names := make([]string, 0, len(e.Rejections))
	for name := range e.Rejections {
		names = append(names, name)
	}
	slices.Sort(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, e.Rejections[name]))
	}

	return fmt.Sprintf(
		"names: no valid candidate after %d attempts (rejected by %s)",
		e.Attempts, strings.Join(parts, ", "),
	)
}

// Unwrap returns ErrNoValidCandidate.
func (e *NoValidCandidateError) Unwrap() error { return ErrNoValidCandidate }
