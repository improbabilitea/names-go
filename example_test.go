package names_test

import (
	"context"
	"fmt"
	"log"
	"testing/fstest"

	names "github.com/improbabilitea/names-go"
	"github.com/improbabilitea/names-go/filters/reserved"
	"github.com/improbabilitea/names-go/packs/cinema"
	"github.com/improbabilitea/names-go/packs/common"
)

// The examples use a seed so their output is fixed. Real code usually omits it.

func Example() {
	generator, err := names.New(names.WithPack(common.Pack))
	if err != nil {
		log.Fatal(err)
	}

	result, err := generator.Generate(names.Seed("42"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Value)
	// Output: zealousvole
}

// Example_themePack composes a theme pack over the base one. The weights decide
// how strongly the theme comes through: at 1 and 4, four names in five use a
// cinema template.
func Example_themePack() {
	generator, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
	)
	if err != nil {
		log.Fatal(err)
	}

	results, err := generator.GenerateN(5, names.Seed("premiere"))
	if err != nil {
		log.Fatal(err)
	}

	for _, r := range results {
		fmt.Println(r.Value)
	}
	// Output:
	// eternalpreviz
	// fabledlobby
	// matineedonkey
	// winsomegreenroom
	// fecundtake
}

// Example_formats shows one logical name rendered several ways. The seed fixes
// the word selection; the format only decides the rendering.
func Example_formats() {
	generator, err := names.New(names.WithPack(common.Pack))
	if err != nil {
		log.Fatal(err)
	}

	for _, format := range []names.Format{
		names.Username, names.Slug, names.Snake, names.Pascal, names.DisplayName,
	} {
		result, err := generator.Generate(names.Seed("format"), names.UseFormat(format))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-12s %s\n", format, result.Value)
	}
	// Output:
	// username     intrepidalbedo
	// slug         intrepid-albedo
	// snake        intrepid_albedo
	// pascal       IntrepidAlbedo
	// display-name Intrepid Albedo
}

// Example_constraints applies a realistic username policy. New validates it up
// front, so a policy no vocabulary could satisfy fails here rather than on a
// user's first request.
func Example_constraints() {
	generator, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(cinema.Pack, names.PackWeight(4)),
		names.WithFormat(names.Username),
		names.WithMinLength(5),
		names.WithMaxLength(20),
		names.WithCharset(names.CharsetLowerAlnum),
		names.WithReservedWords(reserved.Default),
	)
	if err != nil {
		log.Fatal(err)
	}

	result, err := generator.Generate(names.Seed("user_01893"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Value)
	// Output: statelyoberon
}

// Example_customPack loads a pack the library does not ship. Any fs.FS works, so
// the same call handles an embed.FS baked into a binary and a directory on disk;
// fstest.MapFS stands in for both here.
//
// Nothing in the engine changes to accommodate a new pack — which is the point.
func Example_customPack() {
	pack, err := names.LoadFS(fstest.MapFS{
		"pack.json": &fstest.MapFile{Data: []byte(`{
			"name": "brewing",
			"dictionaries": { "brew_noun": "brew_nouns.txt" },
			"templates": [
				{ "pattern": ["adjective", "brew_noun"], "weight": 3 },
				{ "pattern": ["color", "brew_noun"], "weight": 1 }
			]
		}`)},
		"brew_nouns.txt": &fstest.MapFile{Data: []byte("kettle\nmalt\nsteep\nteapot\ninfusion\n")},
	})
	if err != nil {
		log.Fatal(err)
	}

	generator, err := names.New(
		names.WithPack(common.Pack, names.PackWeight(1)),
		names.WithPack(pack, names.PackWeight(5)),
	)
	if err != nil {
		log.Fatal(err)
	}

	results, err := generator.GenerateN(4, names.Seed("brewing"))
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Value)
	}
	// Output:
	// walnutsteep
	// alabasterkettle
	// watchfulmalt
	// gleamingkettle
}

// Example_deterministic shows the seeded guarantee, and its boundary. The same
// seed gives the same name for as long as the fingerprint holds; a vocabulary
// change moves the fingerprint and re-rolls every seeded name.
func Example_deterministic() {
	generator, err := names.New(names.WithPack(common.Pack))
	if err != nil {
		log.Fatal(err)
	}

	for range 3 {
		result, err := generator.Generate(names.Seed("user_01893"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(result.Value)
	}
	// Output:
	// crimsonosprey
	// crimsonosprey
	// crimsonosprey
}

// ExampleGenerator_GenerateUnique delegates the availability question to the
// caller, so the engine never learns what a database is.
//
// Prefer insert-and-retry against a unique constraint for the real thing: check
// then write is a race.
func ExampleGenerator_GenerateUnique() {
	generator, err := names.New(names.WithPack(common.Pack))
	if err != nil {
		log.Fatal(err)
	}

	taken := map[string]bool{"crimsonosprey": true}

	result, err := generator.GenerateUnique(
		context.Background(),
		func(_ context.Context, candidate string) (bool, error) {
			return !taken[candidate], nil
		},
		names.Seed("user_01893"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s after %d attempts\n", result.Value, result.Metadata.Attempts)
	// Output: gildedcricket after 2 attempts
}

// ExampleGenerator_Fingerprint records what seeded output depends on. Store it
// with anything derived from a seed, so a vocabulary change is detected rather
// than silently producing different names.
func ExampleGenerator_Fingerprint() {
	generator, err := names.New(names.WithPack(common.Pack))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(generator.Fingerprint()[:14] + "...")
	// Output: sha256:45b6ac8...
}
