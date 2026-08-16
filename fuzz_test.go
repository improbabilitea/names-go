package names_test

import (
	"regexp"
	"sync"
	"testing"
	"testing/fstest"

	names "github.com/improbabilitea/names-go"
	"github.com/improbabilitea/names-go/filters/reserved"
	"github.com/improbabilitea/names-go/packs/cinema"
	"github.com/improbabilitea/names-go/packs/common"
)

// FuzzLoadFS drives the loader with arbitrary input.
//
// The loader is the only surface in the library that parses anything it did not
// produce itself: a custom pack can come from disk, written by someone else. So
// the contract under fuzzing is narrow but absolute — for *any* input it either
// returns a pack or an error, and never panics.
//
// Anything it does accept then has to be a pack the rest of the library can
// handle, which is why the body goes on to build a generator from it.
func FuzzLoadFS(f *testing.F) {
	f.Add(`{"name":"p","dictionaries":{"a":"a.txt"},"templates":[{"pattern":["a"]}]}`, "velvet\n")
	f.Add(`{"name":"p","words":{"a":[{"value":"velvet","weight":3}]},"templates":[{"pattern":["a"]}]}`, "")
	f.Add(`{"name":"","templates":[]}`, "")
	f.Add(`{`, "\x00")
	f.Add(`{"name":"p","dictionaries":{"a":"a.txt"},"templates":[{"pattern":["a","a"]}]}`, "# only a comment\n")
	f.Add(`{"name":"p","dictionaries":{"a":"a.txt"},"templates":[{"pattern":["a"]}]}`, "velvet 4294967295\n")

	f.Fuzz(func(t *testing.T, manifest, words string) {
		fsys := fstest.MapFS{
			"pack.json": &fstest.MapFile{Data: []byte(manifest)},
			"a.txt":     &fstest.MapFile{Data: []byte(words)},
		}

		pack, err := names.LoadFS(fsys)
		if err != nil {
			return
		}
		if pack == nil {
			t.Fatal("LoadFS returned a nil pack and a nil error")
		}
		if pack.Name() == "" {
			t.Fatal("LoadFS accepted a pack with an empty name")
		}

		// A pack that loaded must be usable, or at least fail cleanly.
		g, err := names.New(names.WithPack(pack))
		if err != nil {
			return
		}

		result, err := g.Generate(names.Seed("fuzz"))
		if err != nil {
			return
		}
		if result.Value == "" {
			t.Fatalf("Generate returned an empty name from words %v", result.Words)
		}
		if len(result.Words) == 0 {
			t.Fatal("Generate returned a name with no words")
		}
	})
}

var (
	fuzzOnce      sync.Once
	fuzzGenerator *names.Generator
	fuzzValid     = regexp.MustCompile(`^[a-z0-9]{5,20}$`)
)

// FuzzGenerateSeed drives seeded generation with arbitrary seeds.
//
// Two properties are asserted. The first is the one a consumer relies on: a name
// that comes back satisfies the configured policy, whatever the seed contained.
// The second is the determinism contract itself — the same seed twice must give
// the same name, and a fuzzer is better than a fixed list at finding a seed where
// that stops holding.
func FuzzGenerateSeed(f *testing.F) {
	fuzzOnce.Do(buildFuzzGenerator)

	for _, seed := range []string{
		"", "42", "user_01893", "a", "ünïcødé", "\x00\x01", "  ",
		"00000000-0000-0000-0000-000000000000",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, seed string) {
		result, err := fuzzGenerator.Generate(names.Seed(seed))
		if err != nil {
			// Exhausting the attempt budget is a legitimate outcome and, for a
			// fixed seed, a permanent one. It is not a crash.
			return
		}

		if !fuzzValid.MatchString(result.Value) {
			t.Fatalf("seed %q produced %q, which violates the configured policy", seed, result.Value)
		}

		again, err := fuzzGenerator.Generate(names.Seed(seed))
		if err != nil {
			t.Fatalf("seed %q succeeded then failed: %v", seed, err)
		}
		if again.Value != result.Value {
			t.Fatalf("seed %q produced %q then %q", seed, result.Value, again.Value)
		}
	})
}

func buildFuzzGenerator() {
	fuzzGenerator = names.MustNew(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
		names.WithFormat(names.Username),
		names.WithMinLength(5),
		names.WithMaxLength(20),
		names.WithCharset(names.CharsetLowerAlnum),
		names.WithReservedWords(reserved.Default),
	)
}
