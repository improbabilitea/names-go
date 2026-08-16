// Package common provides the generic vocabulary pack.
//
// It is the base every other pack builds on: theme packs contribute their own
// nouns but lean on `adjective`, `color` and `mood` from here, so a generator
// configured with a theme pack alone will fail to construct.
//
//	generator, err := names.New(
//	    names.WithPack(common.Pack),
//	)
//
// The pack is embedded in the binary, so nothing needs deploying alongside it.
package common

import (
	"embed"

	names "github.com/improbabilitea/names-go"
)

//go:embed pack.json *.txt
var packFS embed.FS

// Pack is the generic vocabulary: adjectives, animals, colors, moods, nature,
// objects and space, with ten templates over them.
//
// It is parsed once at package initialization. A failure here means the embedded
// files are corrupt, which no caller can do anything about and every test in the
// repository would catch, so it panics rather than complicating every consumer
// with an error they cannot handle.
var Pack = mustLoad()

func mustLoad() *names.Pack {
	pack, err := names.LoadFS(packFS)
	if err != nil {
		panic("names/packs/common: embedded pack is invalid: " + err.Error())
	}
	return pack
}
