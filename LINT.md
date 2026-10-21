# gobuzzkey Lint

Single entry point. `make lint` runs every static check, writes one report per checker under `reports/`, and stops at the first blocking failure. Advisory stages run last and never fail the build.

`make test` is the test-side gate (unit tests with the race detector plus the test timing audit).

```bash
make lint   # all static checks + advisory reports
make test   # unit tests + timing audit
```

`make check` = build + lint + test. Install the pinned tools with `make install`.

## What `make lint` runs

| # | Stage | Tool | Blocking | Catches |
|---|---|---|---|---|
| 1 | Domain purity | `scripts/lint-domain-purity.sh` | yes | A package in the compiled import graph of `domain/...` that is neither stdlib nor `domain/`. Reads the build graph, not configuration, so a relaxed rule still fails here. |
| 2 | Architecture | `go-arch-lint check` + `graph` | yes | Outward dependencies, a component using an undeclared module, a file attached to no component, a component path that does not exist. `deepScan` follows method calls, not only imports. |
| 3 | Planning references | `scripts/lint-planning-refs.sh --self-test` | yes | A planning-document identifier (`Finding 2`, `HIGH-3`, `Chunk 13`, "see the plan", …) in any Go file, tests included. |
| 4 | Format | `gofmt -l` | yes | Unformatted files. Auto-fix: `make lint-fix`. |
| 5 | Vet | `go vet ./...` | yes | Standard library correctness checks. |
| 6 | Lint | `golangci-lint run` | yes | The `.golangci.yml` rule set: `depguard`, `forbidigo`, `wrapcheck`, `ireturn`, `interfacebloat`, `gochecknoglobals`, `gochecknoinits`, `errorlint`, `nolintlint`, `misspell`, `revive`, and the standard set. |
| 7 | Duplicate scan | `dupl -threshold 75` → `reports/dupl.log` | no | Repeated blocks of 75 or more tokens. Prompt for a missing value object or helper. |
| 8 | Repeated literals | `goconst` → `reports/goconst.log` | no | Strings or numbers repeated four or more times. Prompt for a missing constant. |

## Rules golangci-lint enforces here

| Rule | Linter | Scope |
|---|---|---|
| Domain imports no outer layer, no third-party module, no `os` or `net/http` | depguard `domain` | `domain/**` |
| Ports leak no transport or library types | depguard `ports` | `ports/**` |
| App reaches the world only through ports | depguard `app` | `app/**` |
| Adapters never call use cases or the composition root | depguard `adapters` | `adapters/**` |
| btcec only in `adapters/crypto/secp256k1/acl_*.go`; x/term only in `adapters/secret/terminal/acl_*.go` | depguard `btcec`, `xterm` | everywhere |
| No `time.Now`, `time.Sleep`, `time.After`, … | forbidigo | all but `domain/clock/real.go`, tests and `testutil/` |
| No `os.Getenv` / `LookupEnv` / `Environ` | forbidigo | all but `cmd/gobuzzkey/main.go`, tests and `testutil/` |
| No `fmt.Print*`, `print`, `println`: write to an injected `io.Writer` | forbidigo | all but tests and `testutil/` |
| No `SecretKey.Nsec` / `SecretKey.Bytes` | forbidigo | all but `domain/key` (definition), `adapters/store/envfile`, `adapters/crypto/secp256k1`, tests and `testutil/` |
| Ports have at most 4 methods | interfacebloat | everywhere |
| Return concrete types | ireturn | production code |
| `//nolint` names the linter and says why | nolintlint | everywhere |

Tests (`_test.go`) are also exempt from gochecknoglobals, wrapcheck, revive and ireturn; `testutil/` is exempt from forbidigo and revive. Both are test infrastructure, not production code.

## Reading the output

Each stage prints `=== <stage> ===` before its output; the last `===` line of a failed run names the failing stage. Every stage also writes its full output to `reports/<tool>.log`. A stage that did not run keeps the log from its last run, so check the modification time before trusting a report downstream of a failure. CI uploads `reports/` as an artifact on every run.

| Report | Source | Blocking |
|---|---|---|
| `reports/domain-purity.log` | `scripts/lint-domain-purity.sh` | yes |
| `reports/go-arch-lint.log` | `go-arch-lint check` | yes |
| `reports/go-arch-lint-graph.svg` | `go-arch-lint graph` | (diagram) |
| `reports/planning-refs.log` | `scripts/lint-planning-refs.sh` | yes |
| `reports/gofmt.log` | `gofmt -l` | yes |
| `reports/go-vet.log` | `go vet` | yes |
| `reports/golangci.log` | `golangci-lint run` | yes |
| `reports/dupl.log` | `dupl` | no |
| `reports/goconst.log` | `goconst` | no |
| `reports/test.log` | `make test` | yes (test gate) |

## Failure → fix recipes

| Failing check | Report | Fix |
|---|---|---|
| domain purity | `domain-purity.log` | Move the dependency behind a port in `ports/`, implement it in `adapters/`. |
| go-arch-lint outward edge | `go-arch-lint.log` | Move the type inward, introduce a port, or wire it in `cmd/gobuzzkey`. |
| go-arch-lint "not attached to any component" | `go-arch-lint.log` | Add the package as a component (AGENTS.md, "Adding a package"). |
| go-arch-lint vendor | `go-arch-lint.log` | Use the library only in its adapter; declare it under `vendors` and `canUse`. |
| planning identifier | `planning-refs.log` | Write the rule in plain English, or cite an ADR, a root doc section, a NIP or a glossary term. |
| depguard `btcec` / `xterm` | `golangci.log` | Move the import into the adapter's `acl_*.go`; export domain types only. |
| depguard layer rule | `golangci.log` | Same as go-arch-lint; depguard is the per-file mirror. |
| forbidigo `time.*` | `golangci.log` | Take a `clock.Clock`; call `clk.Now()`. |
| forbidigo `os.Getenv` | `golangci.log` | Read the variable in `cmd/gobuzzkey/main.go` (via `env.getenv`) and pass the value in. |
| forbidigo `fmt.Print*` | `golangci.log` | Write to an injected `io.Writer`. |
| forbidigo secret disclosure | `golangci.log` | Do not reveal the key. Pass the `key.SecretKey` to a port that needs it. |
| wrapcheck | `golangci.log` | Wrap with `shared.Wrap(code, msg, err)` so the error carries a `Code`. |
| interfacebloat | `golangci.log` | Split the port; consumers depend on the part they use. |
| gofmt | `gofmt.log` | `make lint-fix`. |
| `audit-test-timings` (in `make test`) | stdout | Replace `time.Sleep` with an injected `clocktest.Fixed` or a channel signal. |

## Authoritative sources

- `.go-arch-lint.yml`: component dependency map, the source of truth for layering.
- `.golangci.yml`: golangci-lint rule set.
- `scripts/lint-domain-purity.sh`, `scripts/lint-planning-refs.sh`.
- `.github/workflows/ci.yml`: CI invocation and report upload.
