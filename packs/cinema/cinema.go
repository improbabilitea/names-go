// Package cinema provides a film and theatre vocabulary pack.
//
// It is a theme pack: it contributes the `cinema_noun` category and templates
// that combine it with `adjective`, `mood`, `color` and `animal` — categories it
// does not define. Register it alongside common, or New fails with
// ErrDictionaryNotFound:
//
//	generator, err := names.New(
//	    names.WithPack(common.Pack, names.PackWeight(1)),
//	    names.WithPack(cinema.Pack, names.PackWeight(4)),
//	)
//
// The weights are how a product decides how strongly the theme comes through.
// At 1 and 4, four names in five use a cinema template.
package cinema

import (
	"embed"

	names "github.com/improbabilitea/names-go"
)

//go:embed pack.json *.txt
var packFS embed.FS

// Pack is the cinema vocabulary. See the package comment for why it cannot be
// used on its own.
var Pack = mustLoad()

func mustLoad() *names.Pack {
	pack, err := names.LoadFS(packFS)
	if err != nil {
		panic("names/packs/cinema: embedded pack is invalid: " + err.Error())
	}
	return pack
}
