package names

import (
	"errors"
	"strings"
	"testing"
)

// TestPackValidationError_ClassifiesAsItsSentinel is the contract that keeps the
// structured types from breaking the convention: callers match on sentinels, and
// only reach for the concrete type when they want the detail.
func TestPackValidationError_ClassifiesAsItsSentinel(t *testing.T) {
	cases := []struct {
		name     string
		sentinel error
	}{
		{"invalid pack", ErrInvalidPack},
		{"invalid template", ErrInvalidTemplate},
		{"invalid weight", ErrInvalidWeight},
		{"empty dictionary", ErrEmptyDictionary},
		{"dictionary not found", ErrDictionaryNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := error(packErrorf(tc.sentinel, "cinema", "templates[0]", "boom"))

			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("errors.Is(err, %v) = false", tc.sentinel)
			}
			if errors.Is(err, ErrNoPack) {
				t.Fatal("error matched an unrelated sentinel")
			}
		})
	}
}

func TestPackValidationError_ExposesDetailViaErrorsAs(t *testing.T) {
	err := error(packErrorf(ErrInvalidWeight, "cinema", "templates[2].weight", "must be greater than zero"))

	var detail *PackValidationError
	if !errors.As(err, &detail) {
		t.Fatal("errors.As did not yield a *PackValidationError")
	}

	if detail.Pack != "cinema" {
		t.Errorf("Pack = %q, want %q", detail.Pack, "cinema")
	}
	if detail.Field != "templates[2].weight" {
		t.Errorf("Field = %q, want %q", detail.Field, "templates[2].weight")
	}
	if detail.Reason != "must be greater than zero" {
		t.Errorf("Reason = %q", detail.Reason)
	}
}

// TestPackValidationError_MessageLocatesTheProblem guards the property that
// makes load-time validation worth having: the message alone has to be enough to
// fix the manifest.
func TestPackValidationError_MessageLocatesTheProblem(t *testing.T) {
	err := packErrorf(ErrInvalidWeight, "cinema", "templates[2].weight", "must be greater than zero")

	got := err.Error()
	for _, want := range []string{"names:", `"cinema"`, "templates[2].weight", "must be greater than zero"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q does not mention %q", got, want)
		}
	}
}

func TestPackValidationError_MessageWithoutField(t *testing.T) {
	err := packErrorf(ErrInvalidPack, "pack.json", "", "unreadable manifest")

	if got, want := err.Error(), `names: pack "pack.json": unreadable manifest`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestNoValidCandidateError_ClassifiesAsItsSentinel(t *testing.T) {
	err := error(&NoValidCandidateError{Attempts: 100})

	if !errors.Is(err, ErrNoValidCandidate) {
		t.Fatal("errors.Is(err, ErrNoValidCandidate) = false")
	}

	var detail *NoValidCandidateError
	if !errors.As(err, &detail) {
		t.Fatal("errors.As did not yield a *NoValidCandidateError")
	}
	if detail.Attempts != 100 {
		t.Errorf("Attempts = %d, want 100", detail.Attempts)
	}
}

// TestNoValidCandidateError_MessageIsStable matters more than it looks: a map
// range would reorder the breakdown between two identical runs, so a golden test
// or a log-diff would show spurious changes in a library whose whole selling
// point is reproducibility.
func TestNoValidCandidateError_MessageIsStable(t *testing.T) {
	newErr := func() error {
		return &NoValidCandidateError{
			Attempts:   100,
			Rejections: map[string]int{"reserved": 13, "length": 80, "charset": 7},
		}
	}

	want := "names: no valid candidate after 100 attempts (rejected by charset=7, length=80, reserved=13)"
	for range 20 {
		if got := newErr().Error(); got != want {
			t.Fatalf("Error() = %q, want %q", got, want)
		}
	}
}

func TestNoValidCandidateError_MessageWithoutRejections(t *testing.T) {
	err := &NoValidCandidateError{Attempts: 7}

	if got, want := err.Error(), "names: no valid candidate after 7 attempts"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// TestSentinels_AreDistinct guards against a copy-paste that would make two
// different failures indistinguishable to errors.Is.
func TestSentinels_AreDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrNoPack":             ErrNoPack,
		"ErrNoTemplate":         ErrNoTemplate,
		"ErrDictionaryNotFound": ErrDictionaryNotFound,
		"ErrInvalidPack":        ErrInvalidPack,
		"ErrInvalidTemplate":    ErrInvalidTemplate,
		"ErrInvalidWeight":      ErrInvalidWeight,
		"ErrEmptyDictionary":    ErrEmptyDictionary,
		"ErrNoValidCandidate":   ErrNoValidCandidate,
		"ErrInvalidOptions":     ErrInvalidOptions,
		"ErrUnsupportedFormat":  ErrUnsupportedFormat,
	}

	seen := make(map[string]string, len(sentinels))
	for name, err := range sentinels {
		message := err.Error()

		if !strings.HasPrefix(message, "names: ") {
			t.Errorf("%s: message %q lacks the package prefix", name, message)
		}
		if other, dup := seen[message]; dup {
			t.Errorf("%s and %s share the message %q", other, name, message)
		}
		seen[message] = name
	}
}
