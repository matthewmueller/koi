# AGENTS.md

Koi is a Go library (no binary, no services, no env vars). Needs Go 1.27+ (generic methods).

- Setup: `go version` must be ≥ 1.27. If toolchain auto-download fails, run `mkdir -p /usr/local/go1.27 && curl -sSL https://dl.google.com/go/go1.27.0.linux-amd64.tar.gz | tar -C /usr/local/go1.27 --strip-components=1 -xz && export PATH=/usr/local/go1.27/bin:$PATH GOTOOLCHAIN=local`, then `go mod download`.
- Build: `go build ./...`
- Test: `go test -race ./...` (Linux needs cgo + gcc for `-race`; `make test` sets `CGO_ENABLED=0` so it fails on Linux).
- Lint: `gofmt -l .`, `go vet ./...`, `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...`, `go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@v0.53.0 -test ./...` (add `-fix` to apply).
- Run: nothing to run; see the example in `Readme.md`.
- CI: `.github/workflows/ci.yml` runs the lint and test commands above on every push.
- No generated code. `error.gohtml` is embedded via `go:embed`.
- Add user-facing changes to the top of `Changelog.md`.

Never run `make release`: it commits, tags, pushes to `main` and creates a GitHub release. Don't push tags or to `main`.
