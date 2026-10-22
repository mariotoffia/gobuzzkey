---
name: code-review
description: Review gobuzzkey pull requests and diffs. Use when asked to review a PR, a branch, or staged changes in this repository. Covers what to check, what lint already enforces, what not to suggest, and which path-scoped instruction files hold the rules for each area.
---

# Reviewing a gobuzzkey change

gobuzzkey is a Go command-line tool that mints owner-attested (NIP-OA) agent identities for Buzz relays. It is built on Hexagonal + DDD + Clean Architecture. `make lint` and `make test` already enforce layering, library confinement, time and environment access, secret disclosure and style. A review is worth reading only when it finds what they cannot: wrong behaviour, a broken wire contract, a leaked secret, a missing test, or docs that promise more than the code does.

## Step 0: load the rules for the touched paths

| Changed path | Read |
|---|---|
| `domain/**`, `ports/**` | `.github/instructions/domain-ports.instructions.md` |
| `app/**` | `.github/instructions/app.instructions.md` |
| `adapters/**` | `.github/instructions/adapters.instructions.md` |
| `cmd/**` | `.github/instructions/cmd.instructions.md` |
| `*_test.go`, `tests/**`, `testutil/**` | `.github/instructions/tests.instructions.md` |
| `.github/workflows/**`, `Makefile`, `CHANGELOG.md`, `go.mod` | `.github/instructions/release.instructions.md` |

## Step 1: understand the intent

- Read the PR description and any linked issue. A decision recorded in an ADR under `docs/adr/` is not reopened in review.
- Read the whole changed file and the callers of any changed function, not only the hunk.

## Step 2: what lint already enforces — do not flag

Layering (`.go-arch-lint.yml`, depguard, `scripts/lint-domain-purity.sh`), library imports outside `acl_*.go`, `time.Now`/`time.Sleep` outside `domain/clock/real.go`, `os.Getenv` outside `cmd/gobuzzkey/main.go`, `fmt.Print*`, `SecretKey.Nsec`/`Bytes` outside the two allowed adapters, `time.Sleep` in tests, planning identifiers in Go files, gofmt, go vet and the golangci-lint rule set. If one of these is broken, CI is red; at most name the checker (`LINT.md`).

Compile errors and import cycles belong to the compiler. Do not claim a file will not build.

## Step 3: what to check

1. **Secrets stay secret, output stays inert.** No new path writes a secret key to output, a log, an error message, a URL or a file other than through the identity store (the identity file, `-out -`, and the Grok Bot prompt, ADR-0004). No flag accepts a secret key as an argument. Relay- or file-supplied text shown to the user goes through `shared.Printable`.
2. **Wire contracts hold.** The NIP-OA preimage is `"nostr:agent-auth:" + agent-hex + ":" + conditions`, conditions are kept verbatim, wire hex is lowercase, the tag has exactly four elements. NIP-98 events carry `u` (absolute URL exactly as requested), `method`, `nonce` and `payload` for bodies. Event IDs use `nostr.Serialize`, never `encoding/json`. Relay URLs carry no path.
3. **Fail closed.** Malformed keys, tags, conditions, URLs and identity files are rejected with a classified `shared.Code`, never defaulted. An existing identity file is never overwritten; `new` keeps it only when `Inspector.Reusable` accepts it.
4. **Order of operations in mint.** Everything that can fail without the owner's key (relay URL, expiry, destination) is checked before the key is requested. The owner key signs the attestation and nothing else: channel joins and leaves are signed by the agent (ADR-0004).
5. **Codes are complete.** A new `shared.Code` has an exit status in `cmd/gobuzzkey/exit.go`, a row in `UBIQUITOUS.md` and in the README's exit-status table, and `TestExitCode_MapsEveryErrorCode` lists it.
6. **The fake relay matches Buzz.** A change to admission behaviour in `adapters/relay/buzzhttp` is mirrored in `testutil/fakerelay`, and both match `ARCHITECTURE.md §4`. The fake also keeps Buzz's channel rules: open channels are visible to every admitted caller and joinable, private ones only to members; only members post.
7. **Docs and code agree.** README flags, defaults and exit statuses match the code. A user-visible change has an entry under `## [Unreleased]` in `CHANGELOG.md`.
8. **Tests pin behaviour.** A bug fix comes with a test that fails without it. Specification vectors are not edited (`TESTS.md §4`).

## Step 4: what not to suggest

- Moving signing into the domain, or adding a crypto library to it (ADR-0002).
- Offering kind conditions in `new` (ADR-0001).
- Adding a flag that takes a secret key as an argument.
- Mocking frameworks; fakes are hand-written (`TESTS.md §3`).
- Restating a lint rule as a finding.

## Output format

```text
<file>:<line> <BLOCKER|HIGH|MEDIUM|LOW|NIT> <correctness|security|architecture|test-gap|clarity>: <problem>. <required fix>.
```

If there are no findings: `No findings.`
