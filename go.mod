module github.com/improbabilitea/names-go

go 1.26.4

// v1.0.0 was tagged by the release pipeline before this module was ready to be
// published, and the module mirror cached it within minutes. The mirror does not
// remove cached versions — deleting the repository does not help — so the
// version is withdrawn here instead: the go command and pkg.go.dev both skip
// retracted versions, and reaching it now requires pinning it deliberately.
retract v1.0.0
