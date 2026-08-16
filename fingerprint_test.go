package names

import (
	"strings"
	"testing"
)

func fingerprintOf(t *testing.T, packs ...registeredPack) fingerprint {
	t.Helper()
	return computeFingerprint(packs)
}

func registered(pack *Pack, weight uint32) registeredPack {
	return registeredPack{pack: pack, weight: weight}
}

// assertSameFingerprint is for edits that are cosmetic: they must leave every
// seeded name in existence exactly where it was.
func assertSameFingerprint(t *testing.T, before, after fingerprint, edit string) {
	t.Helper()
	if before != after {
		t.Fatalf("%s changed the fingerprint (%s -> %s); seeded output would have been re-rolled",
			edit, before, after)
	}
}

// assertDifferentFingerprint is for edits that do reach the output: the
// fingerprint has to move, or a consumer's golden test would not notice the
// vocabulary changed under it.
func assertDifferentFingerprint(t *testing.T, before, after fingerprint, edit string) {
	t.Helper()
	if before == after {
		t.Fatalf("%s left the fingerprint unchanged (%s); the change would go undetected", edit, before)
	}
}

// TestFingerprint_IgnoresSourceOrder is the property that makes vocabulary
// curation safe. Sorting a .txt, or appending a word and re-sorting the file,
// must not re-roll every seeded name in production.
func TestFingerprint_IgnoresSourceOrder(t *testing.T) {
	base := commonFiles()

	shuffled := commonFiles()
	shuffled["adjectives.txt"] = "silent\ngolden\nvelvet\n"
	shuffled["animals.txt"] = "otter\nfox\n"

	before := fingerprintOf(t, registered(mustLoad(t, base), 1))
	after := fingerprintOf(t, registered(mustLoad(t, shuffled), 1))

	assertSameFingerprint(t, before, after, "reordering the lines of a dictionary file")
}

func TestFingerprint_IgnoresJSONFormatting(t *testing.T) {
	base := commonFiles()

	compact := commonFiles()
	compact["pack.json"] = `{"name":"common","version":"1.0.0",` +
		`"dictionaries":{"animal":"animals.txt","adjective":"adjectives.txt"},` +
		`"templates":[{"pattern":["adjective","animal"],"weight":5}]}`

	before := fingerprintOf(t, registered(mustLoad(t, base), 1))
	after := fingerprintOf(t, registered(mustLoad(t, compact), 1))

	assertSameFingerprint(t, before, after, "reformatting the manifest and reordering its keys")
}

// TestFingerprint_IgnoresPackVersion keeps the fingerprint content-derived. The
// declared version is documentation; bumping it should not silently rename every
// user who was given a seeded name.
func TestFingerprint_IgnoresPackVersion(t *testing.T) {
	base := commonFiles()

	bumped := commonFiles()
	bumped["pack.json"] = strings.Replace(commonManifest, `"version": "1.0.0"`, `"version": "2.0.0"`, 1)

	before := fingerprintOf(t, registered(mustLoad(t, base), 1))
	after := fingerprintOf(t, registered(mustLoad(t, bumped), 1))

	assertSameFingerprint(t, before, after, "bumping the declared pack version")
}

// TestFingerprint_IgnoresTagsAndLocale documents a deliberate v1 exclusion:
// these fields are carried but take no part in selection, so enriching them is
// free. When a future version selects on them they join the fingerprint, and
// that release re-rolls seeded names by design.
func TestFingerprint_IgnoresTagsAndLocale(t *testing.T) {
	plain := map[string]string{
		"pack.json": `{"name":"common","words":{"adjective":[{"value":"velvet"}]},` +
			`"templates":[{"pattern":["adjective"]}]}`,
	}
	enriched := map[string]string{
		"pack.json": `{"name":"common","words":{"adjective":[` +
			`{"value":"velvet","tags":["visual","soft"],"locale":"en"}]},` +
			`"templates":[{"pattern":["adjective"]}]}`,
	}

	before := fingerprintOf(t, registered(mustLoad(t, plain), 1))
	after := fingerprintOf(t, registered(mustLoad(t, enriched), 1))

	assertSameFingerprint(t, before, after, "adding tags and a locale")
}

func TestFingerprint_IgnoresTemplateOrder(t *testing.T) {
	one := map[string]string{
		"pack.json": `{"name":"common","dictionaries":{"adjective":"a.txt"},"templates":[` +
			`{"pattern":["adjective"],"weight":3},{"pattern":["adjective","adjective"],"weight":1}]}`,
		"a.txt": "velvet\n",
	}
	other := map[string]string{
		"pack.json": `{"name":"common","dictionaries":{"adjective":"a.txt"},"templates":[` +
			`{"pattern":["adjective","adjective"],"weight":1},{"pattern":["adjective"],"weight":3}]}`,
		"a.txt": "velvet\n",
	}

	before := fingerprintOf(t, registered(mustLoad(t, one), 1))
	after := fingerprintOf(t, registered(mustLoad(t, other), 1))

	assertSameFingerprint(t, before, after, "reordering the templates array")
}

// TestFingerprint_DisabledWordIsAbsent checks that disabling a word is
// equivalent to deleting it. A word that cannot be selected is, as far as the
// output is concerned, not in the pack.
func TestFingerprint_DisabledWordIsAbsent(t *testing.T) {
	withDisabled := map[string]string{
		"pack.json": `{"name":"common","words":{"adjective":[` +
			`{"value":"velvet"},{"value":"retired","enabled":false}]},` +
			`"templates":[{"pattern":["adjective"]}]}`,
	}
	without := map[string]string{
		"pack.json": `{"name":"common","words":{"adjective":[{"value":"velvet"}]},` +
			`"templates":[{"pattern":["adjective"]}]}`,
	}

	before := fingerprintOf(t, registered(mustLoad(t, withDisabled), 1))
	after := fingerprintOf(t, registered(mustLoad(t, without), 1))

	assertSameFingerprint(t, before, after, "disabling a word rather than deleting it")
}

func TestFingerprint_ChangesWith(t *testing.T) {
	base := fingerprintOf(t, registered(mustLoad(t, commonFiles()), 1))

	cases := []struct {
		edit  string
		build func() fingerprint
	}{
		{
			edit: "adding a word",
			build: func() fingerprint {
				files := commonFiles()
				files["adjectives.txt"] = "velvet\nsilent\ngolden\ncosmic\n"
				return fingerprintOf(t, registered(mustLoad(t, files), 1))
			},
		},
		{
			edit: "changing a word weight",
			build: func() fingerprint {
				files := commonFiles()
				files["adjectives.txt"] = "velvet 2\nsilent\ngolden\n"
				return fingerprintOf(t, registered(mustLoad(t, files), 1))
			},
		},
		{
			edit: "changing a template weight",
			build: func() fingerprint {
				files := commonFiles()
				files["pack.json"] = strings.Replace(commonManifest, `"weight": 5`, `"weight": 6`, 1)
				return fingerprintOf(t, registered(mustLoad(t, files), 1))
			},
		},
		{
			edit: "renaming the pack",
			build: func() fingerprint {
				files := commonFiles()
				files["pack.json"] = strings.Replace(commonManifest, `"name": "common"`, `"name": "base"`, 1)
				return fingerprintOf(t, registered(mustLoad(t, files), 1))
			},
		},
		{
			edit: "changing the pack weight",
			build: func() fingerprint {
				return fingerprintOf(t, registered(mustLoad(t, commonFiles()), 4))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.edit, func(t *testing.T) {
			assertDifferentFingerprint(t, base, tc.build(), tc.edit)
		})
	}
}

func TestFingerprint_ChangesWithRegistrationOrder(t *testing.T) {
	common := mustLoad(t, commonFiles())
	cinema := mustLoad(t, map[string]string{
		"pack.json":        `{"name":"cinema","dictionaries":{"cinema_noun":"cinema_nouns.txt"},"templates":[{"pattern":["adjective","cinema_noun"]}]}`,
		"cinema_nouns.txt": "frame\nreel\n",
	})

	before := fingerprintOf(t, registered(common, 1), registered(cinema, 4))
	after := fingerprintOf(t, registered(cinema, 4), registered(common, 1))

	assertDifferentFingerprint(t, before, after, "swapping the registration order")
}

// TestFingerprint_LengthPrefixDisambiguates is why every field is written with
// its length. Without it these two vocabularies both serialize to "abc" and
// would share a fingerprint — and therefore a seeded stream.
func TestFingerprint_LengthPrefixDisambiguates(t *testing.T) {
	build := func(first, second string) fingerprint {
		files := map[string]string{
			"pack.json": `{"name":"p","dictionaries":{"c":"c.txt"},"templates":[{"pattern":["c"]}]}`,
			"c.txt":     first + "\n" + second + "\n",
		}
		return fingerprintOf(t, registered(mustLoad(t, files), 1))
	}

	if build("ab", "c") == build("a", "bc") {
		t.Fatal(`the vocabularies {"ab","c"} and {"a","bc"} share a fingerprint; the length prefix is not doing its job`)
	}
}

func TestFingerprint_StringIsPrefixedAndStable(t *testing.T) {
	got := fingerprintOf(t, registered(mustLoad(t, commonFiles()), 1)).String()

	if !strings.HasPrefix(got, "sha256:") {
		t.Errorf("String() = %q, want a sha256: prefix", got)
	}
	if len(got) != len("sha256:")+64 {
		t.Errorf("String() = %q, want 64 hex characters after the prefix", got)
	}
}

func TestFingerprint_IsDeterministic(t *testing.T) {
	first := fingerprintOf(t, registered(mustLoad(t, commonFiles()), 1))

	for range 10 {
		if got := fingerprintOf(t, registered(mustLoad(t, commonFiles()), 1)); got != first {
			t.Fatalf("fingerprint varied between runs: %s then %s", first, got)
		}
	}
}
