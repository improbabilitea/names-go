package names

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// manifestName is the file every pack is identified by.
const manifestName = "pack.json"

// manifest is the on-disk shape of pack.json.
//
// JSON rather than YAML: the core must stay on the standard library, and the
// readability argument for YAML would only apply to this one file — the words
// themselves live in .txt, where neither format has anything to offer.
type manifest struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Dictionaries map[string]string      `json:"dictionaries"`
	Words        map[string][]wordEntry `json:"words"`
	Templates    []templateEntry        `json:"templates"`
}

// wordEntry is a word declared inline, for the cases that need metadata a .txt
// line cannot carry.
//
// Weight and Enabled are pointers so the loader can tell "absent" from "zero"
// and "false". Absent means the default; an explicit zero weight is a mistake
// worth reporting rather than a silent way to disable a word.
type wordEntry struct {
	Value   string   `json:"value"`
	Tags    []string `json:"tags"`
	Weight  *uint32  `json:"weight"`
	Locale  string   `json:"locale"`
	Enabled *bool    `json:"enabled"`
}

type templateEntry struct {
	Pattern []string `json:"pattern"`
	Weight  *uint32  `json:"weight"`
}

// LoadFS reads and validates a pack from a filesystem.
//
// One function covers both delivery mechanisms the library promises — packs
// embedded in the binary and packs loaded from disk — because embed.FS and
// os.DirFS are both fs.FS:
//
//	names.LoadFS(embeddedPackFS)
//	names.LoadFS(os.DirFS("/etc/improbabilitea/packs/custom"))
//
// Validation here is deliberately *local*. A theme pack legitimately references
// categories it does not define — a cinema pack builds on `adjective`, which
// lives in the common pack — so a pack that is incomplete on its own is not an
// error yet. The cross-pack check happens in New, once the full set is known.
func LoadFS(fsys fs.FS) (*Pack, error) {
	raw, err := fs.ReadFile(fsys, manifestName)
	if err != nil {
		return nil, packErrorf(ErrInvalidPack, manifestName, "", "cannot read manifest: %v", err)
	}

	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// A misspelled key is a silent no-op otherwise, and the pack author would
	// find out only from names that never contain the words they added.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return nil, packErrorf(ErrInvalidPack, manifestName, "", "invalid JSON: %v", err)
	}

	if strings.TrimSpace(m.Name) == "" {
		return nil, packErrorf(ErrInvalidPack, manifestName, "name", "must not be empty")
	}

	dictionaries, err := loadDictionaries(fsys, &m)
	if err != nil {
		return nil, err
	}

	templates, err := loadTemplates(&m)
	if err != nil {
		return nil, err
	}

	return &Pack{
		name:         m.Name,
		version:      m.Version,
		dictionaries: dictionaries,
		templates:    templates,
	}, nil
}

// loadDictionaries assembles every category the manifest declares, from files
// and from inline entries alike.
func loadDictionaries(fsys fs.FS, m *manifest) ([]Dictionary, error) {
	byCategory := make(map[string][]Word)

	// Sorted so that, when several categories are malformed, the reported one is
	// always the same. Diagnosing a pack is hard enough without the error
	// changing between runs.
	for _, category := range sortedKeys(m.Dictionaries) {
		words, err := loadDictionaryFile(fsys, m.Name, category, m.Dictionaries[category])
		if err != nil {
			return nil, err
		}
		byCategory[category] = append(byCategory[category], words...)
	}

	for _, category := range sortedKeys(m.Words) {
		field := fmt.Sprintf("words.%s", category)
		for i, entry := range m.Words[category] {
			word, err := entry.toWord(m.Name, category, fmt.Sprintf("%s[%d]", field, i))
			if err != nil {
				return nil, err
			}
			byCategory[category] = append(byCategory[category], word)
		}
	}

	dictionaries := make([]Dictionary, 0, len(byCategory))
	for _, category := range sortedKeys(byCategory) {
		if category == "" {
			return nil, packErrorf(ErrInvalidPack, m.Name, "dictionaries", "category name must not be empty")
		}

		words := byCategory[category]
		if len(words) == 0 {
			return nil, packErrorf(ErrEmptyDictionary, m.Name,
				fmt.Sprintf("dictionaries.%s", category), "contains no word")
		}

		sortWordsCanonical(words)
		if err := rejectDuplicates(m.Name, category, words); err != nil {
			return nil, err
		}

		dictionaries = append(dictionaries, Dictionary{Category: category, Words: words})
	}

	return dictionaries, nil
}

// loadDictionaryFile parses one .txt: a word per line, an optional weight after
// a space, `#` for comments.
func loadDictionaryFile(fsys fs.FS, pack, category, filename string) ([]Word, error) {
	field := fmt.Sprintf("dictionaries.%s", category)

	raw, err := fs.ReadFile(fsys, filename)
	if err != nil {
		return nil, packErrorf(ErrInvalidPack, pack, field, "cannot read %q: %v", filename, err)
	}
	if !utf8.Valid(raw) {
		return nil, packErrorf(ErrInvalidPack, pack, field, "%q is not valid UTF-8", filename)
	}

	var words []Word
	for i, line := range strings.Split(string(raw), "\n") {
		lineField := fmt.Sprintf("%s:%d", filename, i+1)

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		value, weight, err := parseDictionaryLine(pack, lineField, line)
		if err != nil {
			return nil, err
		}
		if err := validateWordValue(pack, lineField, value); err != nil {
			return nil, err
		}

		words = append(words, Word{
			Value:    value,
			Category: category,
			Weight:   weight,
			Enabled:  true,
		})
	}

	return words, nil
}

// parseDictionaryLine splits a line into its value and optional weight.
func parseDictionaryLine(pack, field, line string) (string, uint32, error) {
	value, weightText, hasWeight := strings.Cut(line, " ")
	if !hasWeight {
		return line, defaultWeight, nil
	}

	weightText = strings.TrimSpace(weightText)
	parsed, err := strconv.ParseUint(weightText, 10, 32)
	if err != nil {
		return "", 0, packErrorf(ErrInvalidWeight, pack, field,
			"weight %q is not a number in the uint32 range", weightText)
	}
	if parsed == 0 {
		return "", 0, packErrorf(ErrInvalidWeight, pack, field,
			"weight must be greater than zero (omit it for the default, or set enabled=false to disable the word)")
	}

	return value, uint32(parsed), nil
}

// toWord converts an inline entry, applying the defaults an absent field means.
func (e wordEntry) toWord(pack, category, field string) (Word, error) {
	if err := validateWordValue(pack, field, e.Value); err != nil {
		return Word{}, err
	}

	weight := defaultWeight
	if e.Weight != nil {
		if *e.Weight == 0 {
			return Word{}, packErrorf(ErrInvalidWeight, pack, field+".weight",
				"weight must be greater than zero (omit it for the default, or set enabled=false to disable the word)")
		}
		weight = *e.Weight
	}

	// Absent means enabled. The zero value of a bool would say the opposite, and
	// a word silently missing from generation is the hardest kind of bug to
	// notice in a vocabulary of several hundred entries.
	enabled := true
	if e.Enabled != nil {
		enabled = *e.Enabled
	}

	return Word{
		Value:    e.Value,
		Category: category,
		Tags:     slices.Clone(e.Tags),
		Weight:   weight,
		Locale:   e.Locale,
		Enabled:  enabled,
	}, nil
}

// validateWordValue rejects values that the rest of the pipeline could not
// handle coherently.
//
// It deliberately does not restrict the alphabet: which characters are
// acceptable is a policy question, belonging to the consumer's charset
// constraint, and hard-coding [a-z] here would rule out localized packs before
// they are written. What it does reject is structural — whitespace would make
// the .txt format ambiguous, and control characters have no place in a name.
func validateWordValue(pack, field, value string) error {
	if value == "" {
		return packErrorf(ErrInvalidPack, pack, field, "word value must not be empty")
	}
	if !utf8.ValidString(value) {
		return packErrorf(ErrInvalidPack, pack, field, "word value is not valid UTF-8")
	}

	for _, r := range value {
		switch {
		case unicode.IsSpace(r):
			return packErrorf(ErrInvalidPack, pack, field,
				"word value %q must not contain whitespace", value)
		case unicode.IsControl(r):
			return packErrorf(ErrInvalidPack, pack, field,
				"word value %q must not contain control characters", value)
		}
	}

	return nil
}

// rejectDuplicates fails on a repeated value within one category.
//
// Within a single pack a duplicate is unambiguously a mistake, and a hand-curated
// vocabulary of several hundred words will collect them. Failing the load is the
// only place it gets noticed — at runtime it merely doubles a word's odds, which
// nobody ever sees. Duplicates *across* packs are a different matter, resolved by
// registration order in New.
func rejectDuplicates(pack, category string, sorted []Word) error {
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Value == sorted[i-1].Value {
			return packErrorf(ErrInvalidPack, pack,
				fmt.Sprintf("dictionaries.%s", category),
				"duplicate word %q", sorted[i].Value)
		}
	}
	return nil
}

// loadTemplates validates the declared patterns and puts them in canonical order.
func loadTemplates(m *manifest) ([]Template, error) {
	templates := make([]Template, 0, len(m.Templates))

	for i, entry := range m.Templates {
		field := fmt.Sprintf("templates[%d]", i)

		if len(entry.Pattern) == 0 {
			return nil, packErrorf(ErrInvalidTemplate, m.Name, field+".pattern", "must not be empty")
		}
		for j, category := range entry.Pattern {
			if strings.TrimSpace(category) == "" {
				return nil, packErrorf(ErrInvalidTemplate, m.Name,
					fmt.Sprintf("%s.pattern[%d]", field, j), "category name must not be empty")
			}
		}

		weight := defaultWeight
		if entry.Weight != nil {
			if *entry.Weight == 0 {
				return nil, packErrorf(ErrInvalidWeight, m.Name, field+".weight",
					"weight must be greater than zero")
			}
			weight = *entry.Weight
		}

		templates = append(templates, Template{
			Pattern: slices.Clone(entry.Pattern),
			Weight:  weight,
		})
	}

	sortTemplatesCanonical(templates)
	return templates, nil
}

// sortedKeys returns a map's keys in a stable order.
//
// Every decision that reaches the output has to be taken over a sorted slice
// rather than a map range, or the same inputs would produce different names
// between two runs of the same binary.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
