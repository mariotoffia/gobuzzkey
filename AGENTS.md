# gobuzzkey Agent Rules

Canonical file. `.claude/CLAUDE.md` is a symlink to this. If the two ever disagree, preserve user edits and report it.

gobuzzkey is a Go 1.27+ command-line tool and library that mints owner-attested (NIP-OA) agent identities for Buzz relays and checks that a relay admits them. It is built on Hexagonal + DDD + Clean Architecture, the same way as [gobridge](https://github.com/mariotoffia/gobridge). Layering and secret handling are machine-enforced, so this file is navigation, not a restatement of the rules.

## First read

- `LANGUAGE.md`: communication style. Read once, apply always.

## MUST: Where to look — task → doc

| Doing this | Read |
|---|---|
| Naming a type, field, constant, flag, error code or concept | `UBIQUITOUS.md` (authoritative glossary) |
| Understanding the domain shape or bounded contexts | `DDD.md` |
| Changing layering, dependencies, or adding a package | `ARCHITECTURE.md §2` + `.go-arch-lint.yml` (source of truth) |
| Writing or modifying any test | `TESTS.md` |
| Lint failed; need to know which checker and how to fix | `LINT.md` |
| Local setup, make targets, CI mapping | `DEVELOPMENT.md` |
| Tagging a release, writing a changelog entry | `RELEASE.md` |
| A decision with lasting consequences | `docs/adr/` (add an ADR) |
| How an agent platform consumes the identity | `docs/guides/` |

Markdown is the default doc format. Do not add root-level docs unless instructed.

## Conventions not enforced by lint

Treat these as hard rules.

- Adapter files end with a compile-time interface assertion: `var _ ports.Relay = (*Client)(nil)`.
- Adapter constructors use functional options: `WithXxx(value)`.
- A third-party library is imported only from `acl_*.go` files in its own adapter. depguard enforces this for the libraries in use today; a new library needs a new depguard rule in the same PR.
- Errors that cross a layer are `*shared.Error` with a `shared.Code`. Callers branch with `shared.IsCode`, never on message text. A new code gets an exit status in `cmd/gobuzzkey/exit.go`, a row in `UBIQUITOUS.md` and a row in the README's exit-status table.
- NIP-OA, NIP-01, NIP-19 and NIP-98 behaviour is pinned by the published test vectors. A change that makes a vector test fail is wrong, not the vector.

Everything else (layering, vendor confinement, time injection, environment access, secret disclosure, gofmt, go vet and golangci-lint rules) is enforced by `make lint` and `make test`. Do not restate those rules in review feedback; point at the failing checker.

## MUST: Never print, log or store a secret key

The owner key is used for one signature and then dropped. The agent key leaves the process only through `ports.IdentityStore`. `key.SecretKey` redacts itself in every `fmt` verb; do not work around that. `SecretKey.Nsec` and `SecretKey.Bytes` are callable only from `adapters/store/envfile` and `adapters/crypto/secp256k1` (forbidigo). Never add a flag that takes a secret key as an argument, because arguments end up in shell history and process listings.

## MUST: Never delete `main` or move a release tag

`main` is permanent. A published `vX.Y.Z` tag is never moved or deleted; a broken release is fixed with a new patch release (`RELEASE.md`).

## MUST: Never reference a planning document

Comments, test names, file names and docs MUST NOT carry review or task identifiers such as `HIGH-3`, `finding F2`, `T14`, `round-2` or "see the plan". Planning documents get deleted; the reference outlives them and points at nothing. Reference only what is durable: an ADR (`see ADR-0001`), a canonical root doc plus section (`ARCHITECTURE.md §2`), a NIP by number, or a `UBIQUITOUS.md` term. Otherwise write the rule in plain English: what must hold, and why.

This is machine-enforced for every Go file, tests included: `make lint` runs `scripts/lint-planning-refs.sh`. Read `reports/planning-refs.log` when it is red.

## MUST: Keep the review rules current

Copilot code review reads `.github/skills/code-review/SKILL.md` and the path-scoped `.github/instructions/*.instructions.md` files. Update them in the same PR when you add, move or rename a package, change a rule in `ARCHITECTURE.md`, `DDD.md`, `UBIQUITOUS.md`, `TESTS.md`, `LINT.md` or `RELEASE.md`, add an ADR, or make a checker enforce a rule the review files list (move it to "what lint already enforces").

## How to know you're done

Two commands.

```bash
make lint   # every static check; one report per checker under reports/
make test   # unit tests with the race detector + test timing audit
```

Both green on your branch. Failures that pre-date your work are still your problem on the branch. A user-visible change also has an entry under `## [Unreleased]` in `CHANGELOG.md`.

Keep test output context-efficient: full logs go to `reports/`; report command, status, count and duration for passing runs and read only failure sections.

`make check` runs build + lint + test.

## Adding a package

1. Place it in a real layer and role: domain bounded context, port, use case, adapter or composition root (`ARCHITECTURE.md §2`).
2. Add it as a component in `.go-arch-lint.yml` with precise `mayDependOn` and `canUse`. go-arch-lint fails on any file not attached to a component.
3. A domain package is stdlib-only; `scripts/lint-domain-purity.sh` checks the compiled import graph.
4. An adapter for a new library gets an `acl_*.go` file and a depguard rule confining the library to it.
5. Cover its path in `.github/instructions/` and the skill's Step 0 table.
6. `make lint` must stay green.

If `make lint` conflicts with the code, the code is wrong. Refactor by moving types inward, introducing a port, or pushing wiring to `cmd/gobuzzkey`. Relax `.go-arch-lint.yml` only when an architectural concept is genuinely missing, and document the reason inline in the YAML.
