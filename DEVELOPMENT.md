# Development Guide

## Prerequisites

- Go 1.27 or later. `go.mod` pins `toolchain go1.27.3`, so an older Go downloads it automatically (`GOTOOLCHAIN=auto`, the default).
- `make`, `bash`, `git`.
- The lint tools, installed at the pinned versions with `make install`: golangci-lint v2, go-arch-lint, dupl, goconst, govulncheck.

## Getting started

```bash
git clone https://github.com/mariotoffia/gobuzzkey
cd gobuzzkey
make install
make check     # build + lint + test
```

## Make targets

| Target | Does |
|---|---|
| `make build` | Builds `./gobuzzkey.out`. |
| `make install-cli` | Builds the checkout and installs `gobuzzkey` into `$(go env GOPATH)/bin` (or `GOBIN`), so it runs from any directory. |
| `make test` | Unit tests with the race detector, plus the test timing audit. |
| `make test-live` | Runs `tests/live` against the relay in `GOBUZZKEY_LIVE_ENV`. |
| `make lint` | Every static check; see [LINT.md](LINT.md). |
| `make lint-fix` | `gofmt -w` on every tracked Go file. |
| `make check` | build + lint + test. |
| `make fuzz` | Mutates every fuzz target for `FUZZTIME` (default 30s) each. |
| `make vulncheck` | `govulncheck ./...` at the pinned version, via `go run`. |
| `make dist VERSION=vX.Y.Z` | Cross-compiled archives and checksums in `dist/`. |
| `make release VERSION=vX.Y.Z [CONFIRM=1]` | Release gates, then tag and push ([RELEASE.md](RELEASE.md)). |
| `make help` | Lists all targets. |

## Trying it against a relay

```bash
make build
./gobuzzkey.out new -relay wss://buzz.example.com -out /tmp/agent.env
./gobuzzkey.out verify -env /tmp/agent.env
./gobuzzkey.out check -env /tmp/agent.env
GOBUZZKEY_LIVE_ENV=/tmp/agent.env make test-live
```

`*.env` files are gitignored. Never commit an identity, not even a test one.

## Repository structure

See `ARCHITECTURE.md §6` for what each package does and `ARCHITECTURE.md §2` for what may import what. In short: `domain/` (pure types and rules), `ports/` (interfaces), `app/` (use cases), `adapters/` (one per technology), `cmd/gobuzzkey` (wiring), `testutil/fakerelay` (test relay), `tests/live` (real relay).

## CI

| Workflow | Trigger | Runs |
|---|---|---|
| `ci.yml`, job `lint` | push to `main`, pull request | `make install` tools at pinned versions, `make lint`; uploads `reports/` |
| `ci.yml`, job `test` | push to `main`, pull request | `make test`, `make build`, `govulncheck`; uploads `reports/` |
| `ci.yml`, job `fuzz` | manual dispatch | `make fuzz` with the chosen `FUZZTIME`; uploads crashers |
| `release.yml` | protected `v*` tag creation | gates, `make dist`, GitHub release |

## Dependencies

The module has two direct dependencies, each confined to one adapter: `github.com/btcsuite/btcd/btcec/v2` (BIP-340, `adapters/crypto/secp256k1`) and `golang.org/x/term` (hidden input, `adapters/secret/terminal`). Adding a dependency means adding an adapter, an `acl_*.go` file, a `vendors` entry in `.go-arch-lint.yml` and a depguard rule (AGENTS.md). Update with `go get -u <module>` and `make tidy`, then `make check`.

## Code conventions

- Hexagonal layering and the Dependency Rule: `ARCHITECTURE.md §2`.
- Names come from [UBIQUITOUS.md](UBIQUITOUS.md).
- Errors are `*shared.Error` with a `Code`; wrap library errors with `shared.Wrap`.
- Time comes from an injected `clock.Clock`.
- Output goes to injected writers; the environment is read in `cmd/gobuzzkey/main.go`.
- Tests follow [TESTS.md](TESTS.md).
