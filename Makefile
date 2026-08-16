.PHONY: lint test test-fuzz bench deps tidy

lint:
	golangci-lint run ./...

test:
	go test -race ./...

# Short fuzz pass over the loader (the only surface that parses untrusted input)
# and over seeds. CI runs the same targets; locally this is the pre-commit check.
test-fuzz:
	go test -run=NONE -fuzz=FuzzLoadFS -fuzztime=30s .
	go test -run=NONE -fuzz=FuzzGenerateSeed -fuzztime=30s .

bench:
	go test -bench=. -benchmem ./...

# RNF-09: the core and the built-in packs must have no dependency outside the
# standard library. `go list -m all` prints this module first, so anything on a
# later line is an acquired dependency.
deps:
	@test -z "$$(go list -m all | tail -n +2)" \
		|| { echo "external dependencies found:"; go list -m all | tail -n +2; exit 1; }

tidy:
	go mod tidy
