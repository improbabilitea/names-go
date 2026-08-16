package names

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// mapFS turns a literal file set into a filesystem. Most loader cases need no
// files on disk at all, which keeps the fixtures next to the assertions instead
// of in testdata.
func mapFS(files map[string]string) fstest.MapFS {
	fsys := make(fstest.MapFS, len(files))
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func mustLoad(t *testing.T, files map[string]string) *Pack {
	t.Helper()

	pack, err := LoadFS(mapFS(files))
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	return pack
}

// wantPackError asserts the failure classifies as sentinel and carries a
// locatable field.
func wantPackError(t *testing.T, err error, sentinel error) *PackValidationError {
	t.Helper()

	if err == nil {
		t.Fatal("LoadFS succeeded, want an error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error %v does not classify as %v", err, sentinel)
	}

	var detail *PackValidationError
	if !errors.As(err, &detail) {
		t.Fatalf("error %v is not a *PackValidationError", err)
	}
	if detail.Reason == "" {
		t.Error("error carries no reason")
	}
	return detail
}

func (p *Pack) wordsIn(t *testing.T, category string) []Word {
	t.Helper()

	for _, d := range p.dictionaries {
		if d.Category == category {
			return d.Words
		}
	}
	t.Fatalf("pack has no category %q (has %v)", category, p.Categories())
	return nil
}

const commonManifest = `{
  "name": "common",
  "version": "1.0.0",
  "dictionaries": { "adjective": "adjectives.txt", "animal": "animals.txt" },
  "templates": [ { "pattern": ["adjective", "animal"], "weight": 5 } ]
}`

func commonFiles() map[string]string {
	return map[string]string{
		"pack.json":      commonManifest,
		"adjectives.txt": "velvet\nsilent\ngolden\n",
		"animals.txt":    "fox\notter\n",
	}
}

func TestLoadFS_ReadsManifestAndDictionaries(t *testing.T) {
	pack := mustLoad(t, commonFiles())

	if got := pack.Name(); got != "common" {
		t.Errorf("Name() = %q, want %q", got, "common")
	}
	if got := pack.Version(); got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}

	adjectives := pack.wordsIn(t, "adjective")
	if len(adjectives) != 3 {
		t.Fatalf("adjective has %d words, want 3", len(adjectives))
	}
	if len(pack.templates) != 1 {
		t.Fatalf("pack has %d templates, want 1", len(pack.templates))
	}
	if got := pack.templates[0].Weight; got != 5 {
		t.Errorf("template weight = %d, want 5", got)
	}
}

// TestLoadFS_SortsWordsCanonically is what makes the fingerprint independent of
// how a vocabulary file happens to be written.
func TestLoadFS_SortsWordsCanonically(t *testing.T) {
	pack := mustLoad(t, commonFiles())

	want := []string{"golden", "silent", "velvet"}
	for i, word := range pack.wordsIn(t, "adjective") {
		if word.Value != want[i] {
			t.Errorf("word %d = %q, want %q", i, word.Value, want[i])
		}
	}
}

func TestLoadFS_AppliesDefaults(t *testing.T) {
	pack := mustLoad(t, commonFiles())

	for _, word := range pack.wordsIn(t, "adjective") {
		if word.Weight != 1 {
			t.Errorf("%q: weight = %d, want the default 1", word.Value, word.Weight)
		}
		if !word.Enabled {
			t.Errorf("%q: enabled = false, want the default true", word.Value)
		}
		if word.Category != "adjective" {
			t.Errorf("%q: category = %q", word.Value, word.Category)
		}
	}
}

func TestLoadFS_ParsesWeightsCommentsAndBlankLines(t *testing.T) {
	files := commonFiles()
	files["adjectives.txt"] = "# curated by hand\n\nvelvet 10\nsilent\n\n#golden is on hold\ncosmic 3\n"

	pack := mustLoad(t, files)

	want := map[string]uint32{"cosmic": 3, "silent": 1, "velvet": 10}
	words := pack.wordsIn(t, "adjective")
	if len(words) != len(want) {
		t.Fatalf("got %d words, want %d", len(words), len(want))
	}
	for _, word := range words {
		if want[word.Value] != word.Weight {
			t.Errorf("%q: weight = %d, want %d", word.Value, word.Weight, want[word.Value])
		}
	}
}

// TestLoadFS_ThemePackWithoutItsBaseIsAccepted is the reason validation is split
// in two. A cinema pack builds on `adjective`, which common provides; it is
// incomplete on its own by design, and rejecting it here would make theme packs
// unloadable.
func TestLoadFS_ThemePackWithoutItsBaseIsAccepted(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{
  "name": "cinema",
  "dictionaries": { "cinema_noun": "cinema_nouns.txt" },
  "templates": [ { "pattern": ["adjective", "cinema_noun"], "weight": 5 } ]
}`,
		"cinema_nouns.txt": "frame\nreel\n",
	})

	if got := pack.Name(); got != "cinema" {
		t.Fatalf("Name() = %q", got)
	}
	if got := pack.Categories(); len(got) != 1 || got[0] != "cinema_noun" {
		t.Fatalf("Categories() = %v, want [cinema_noun]", got)
	}
}

func TestLoadFS_InlineWordsCarryMetadata(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{
  "name": "common",
  "words": {
    "adjective": [
      { "value": "velvet", "tags": ["visual", "soft"], "weight": 8, "locale": "en" },
      { "value": "retired", "enabled": false }
    ]
  },
  "templates": [ { "pattern": ["adjective"] } ]
}`,
	})

	words := pack.wordsIn(t, "adjective")
	if len(words) != 2 {
		t.Fatalf("got %d words, want 2", len(words))
	}

	// Canonical order puts "retired" before "velvet".
	retired, velvet := words[0], words[1]

	if retired.Value != "retired" || retired.Enabled {
		t.Errorf("retired = %+v, want it disabled", retired)
	}
	if velvet.Weight != 8 {
		t.Errorf("velvet weight = %d, want 8", velvet.Weight)
	}
	if velvet.Locale != "en" {
		t.Errorf("velvet locale = %q, want %q", velvet.Locale, "en")
	}
	if len(velvet.Tags) != 2 || velvet.Tags[0] != "visual" {
		t.Errorf("velvet tags = %v", velvet.Tags)
	}
}

func TestLoadFS_MergesFileAndInlineWords(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{
  "name": "common",
  "dictionaries": { "adjective": "adjectives.txt" },
  "words": { "adjective": [ { "value": "cosmic", "weight": 2 } ] },
  "templates": [ { "pattern": ["adjective"] } ]
}`,
		"adjectives.txt": "velvet\nsilent\n",
	})

	if got := len(pack.wordsIn(t, "adjective")); got != 3 {
		t.Fatalf("got %d words, want 3 (2 from file + 1 inline)", got)
	}
}

func TestLoadFS_DefaultsTemplateWeight(t *testing.T) {
	pack := mustLoad(t, map[string]string{
		"pack.json": `{
  "name": "common",
  "dictionaries": { "adjective": "adjectives.txt" },
  "templates": [ { "pattern": ["adjective"] } ]
}`,
		"adjectives.txt": "velvet\n",
	})

	if got := pack.templates[0].Weight; got != 1 {
		t.Fatalf("template weight = %d, want the default 1", got)
	}
}

func TestLoadFS_Rejects(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		sentinel  error
		wantField string
	}{
		{
			name:      "missing manifest",
			files:     map[string]string{"adjectives.txt": "velvet\n"},
			sentinel:  ErrInvalidPack,
			wantField: "",
		},
		{
			name:     "malformed JSON",
			files:    map[string]string{"pack.json": `{"name": "common"`},
			sentinel: ErrInvalidPack,
		},
		{
			name:     "unknown manifest field",
			files:    map[string]string{"pack.json": `{"name": "common", "tempaltes": []}`},
			sentinel: ErrInvalidPack,
		},
		{
			name:      "empty pack name",
			files:     map[string]string{"pack.json": `{"name": "  "}`},
			sentinel:  ErrInvalidPack,
			wantField: "name",
		},
		{
			name: "missing dictionary file",
			files: map[string]string{
				"pack.json": `{"name":"common","dictionaries":{"adjective":"nope.txt"}}`,
			},
			sentinel:  ErrInvalidPack,
			wantField: "dictionaries.adjective",
		},
		{
			name: "empty dictionary",
			files: map[string]string{
				"pack.json":      `{"name":"common","dictionaries":{"adjective":"adjectives.txt"}}`,
				"adjectives.txt": "\n# nothing but a comment\n",
			},
			sentinel:  ErrEmptyDictionary,
			wantField: "dictionaries.adjective",
		},
		{
			name: "zero weight in a dictionary file",
			files: map[string]string{
				"pack.json":      `{"name":"common","dictionaries":{"adjective":"adjectives.txt"}}`,
				"adjectives.txt": "velvet 0\n",
			},
			sentinel:  ErrInvalidWeight,
			wantField: "adjectives.txt:1",
		},
		{
			name: "non-numeric weight",
			files: map[string]string{
				"pack.json":      `{"name":"common","dictionaries":{"adjective":"adjectives.txt"}}`,
				"adjectives.txt": "velvet heavy\n",
			},
			sentinel:  ErrInvalidWeight,
			wantField: "adjectives.txt:1",
		},
		{
			name: "zero weight inline",
			files: map[string]string{
				"pack.json": `{"name":"common","words":{"adjective":[{"value":"velvet","weight":0}]}}`,
			},
			sentinel:  ErrInvalidWeight,
			wantField: "words.adjective[0].weight",
		},
		{
			name: "empty word value",
			files: map[string]string{
				"pack.json": `{"name":"common","words":{"adjective":[{"value":""}]}}`,
			},
			sentinel:  ErrInvalidPack,
			wantField: "words.adjective[0]",
		},
		{
			name: "word containing whitespace",
			files: map[string]string{
				"pack.json": `{"name":"common","words":{"adjective":[{"value":"velvet fox"}]}}`,
			},
			sentinel:  ErrInvalidPack,
			wantField: "words.adjective[0]",
		},
		{
			name: "duplicate word",
			files: map[string]string{
				"pack.json":      `{"name":"common","dictionaries":{"adjective":"adjectives.txt"}}`,
				"adjectives.txt": "velvet\nsilent\nvelvet\n",
			},
			sentinel:  ErrInvalidPack,
			wantField: "dictionaries.adjective",
		},
		{
			name: "empty template pattern",
			files: map[string]string{
				"pack.json": `{"name":"common","templates":[{"pattern":[]}]}`,
			},
			sentinel:  ErrInvalidTemplate,
			wantField: "templates[0].pattern",
		},
		{
			name: "blank category in a pattern",
			files: map[string]string{
				"pack.json": `{"name":"common","templates":[{"pattern":["adjective","  "]}]}`,
			},
			sentinel:  ErrInvalidTemplate,
			wantField: "templates[0].pattern[1]",
		},
		{
			name: "zero template weight",
			files: map[string]string{
				"pack.json": `{"name":"common","templates":[{"pattern":["adjective"],"weight":0}]}`,
			},
			sentinel:  ErrInvalidWeight,
			wantField: "templates[0].weight",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFS(mapFS(tc.files))
			detail := wantPackError(t, err, tc.sentinel)

			if tc.wantField != "" && detail.Field != tc.wantField {
				t.Errorf("Field = %q, want %q (message: %s)", detail.Field, tc.wantField, err)
			}
		})
	}
}

func TestLoadFS_RejectsInvalidUTF8(t *testing.T) {
	fsys := fstest.MapFS{
		"pack.json": &fstest.MapFile{
			Data: []byte(`{"name":"common","dictionaries":{"adjective":"adjectives.txt"}}`),
		},
		"adjectives.txt": &fstest.MapFile{Data: []byte{0xff, 0xfe, '\n'}},
	}

	_, err := LoadFS(fsys)
	_ = wantPackError(t, err, ErrInvalidPack)
}

// TestLoadFS_WorksOnDirFS covers the second delivery mechanism. The library
// promises a custom pack can live outside the binary, and a single fs.FS-based
// loader is what makes that free — but only if it is actually exercised against
// a real directory.
func TestLoadFS_WorksOnDirFS(t *testing.T) {
	dir := t.TempDir()
	for name, content := range commonFiles() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	fromDisk, err := LoadFS(os.DirFS(dir))
	if err != nil {
		t.Fatalf("LoadFS(os.DirFS): %v", err)
	}
	fromMemory := mustLoad(t, commonFiles())

	got := computeFingerprint([]registeredPack{{pack: fromDisk, weight: 1}})
	want := computeFingerprint([]registeredPack{{pack: fromMemory, weight: 1}})
	if got != want {
		t.Fatal("the same pack loaded from disk and from memory produced different fingerprints")
	}
}
