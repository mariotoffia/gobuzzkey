# Test Authoring Rules

Contract every new or modified test follows. Goal: tests that never flake, that pin behaviour against the published specifications, and that run fast enough for `make test` to be a trustworthy signal on every save.

If a rule conflicts with an existing test, the rule wins. Rewrite the test when you touch it. Use the terms in [UBIQUITOUS.md](UBIQUITOUS.md) in test names, helpers and messages.

## 1. Two categories

| Category | Lives in | Gate | Run with | Budget |
|---|---|---|---|---|
| **Unit** | `foo_test.go` next to `foo.go`; `cmd/gobuzzkey/main_test.go` for the command end to end | none | `make test` (`-race -short -timeout 120s`) | under 100 ms per test |
| **Live** | `tests/live/` only | skipped under `-short` and unless `GOBUZZKEY_LIVE_ENV` is set | `make test-live` | seconds |

Everything that talks HTTP in a unit test talks to `testutil/fakerelay`, an in-process Buzz relay started on a loopback port. That is still a unit test: no Docker, no outside network, milliseconds. The live test is the only one that reaches a real relay, and it never posts.

## 2. Anti-flake rules

### 2.1 No `time.Sleep`

`make test` fails on any `time.Sleep(` in a `_test.go` file. Wait on a channel, or do not wait at all: everything here is synchronous.

### 2.2 Time is injected

Pass `clocktest.At(unix)` wherever the code takes a `clock.Clock`, including `fakerelay.WithClock`. The fake relay rejects NIP-98 events more than 60 seconds from its own clock, exactly like Buzz, so a test that mixes real and fixed clocks fails loudly instead of intermittently.

### 2.3 No outside network

Only `tests/live` may reach a real host. Unit tests use `fakerelay` or the hand-written fakes in `app/fakes_test.go`.

### 2.4 Randomness is fine where it cannot change the outcome

Fresh keys from `secp256k1.Crypto.Generate` are fine: a test asserts relationships (this tag verifies for that agent), never specific key bytes. Where bytes matter, use the NIP vectors (§4).

### 2.5 Cleanup is registered when the resource is acquired

`defer r.Close()` right after `fakerelay.Start`, `t.TempDir()` for files, `t.Cleanup` for pipes.

## 3. Architectural correctness

- `app` tests use the fakes in `app/fakes_test.go` and never import an adapter. The fake crypto is deliberately not BIP-340: it ties a signature to one key and one message, which is all the use cases may assume.
- Adapter tests use the real library and the real domain types.
- `cmd/gobuzzkey` tests call `run` with an injected environment, clock, stdin and writers, and exercise real adapters against `fakerelay`.
- Fakes implement exactly the port they claim and end with a `var _ ports.X = …` assertion. No mocking frameworks.
- Errors are compared with `shared.IsCode` or `errors.Is`, never by message text.

## 4. Specification vectors are the contract

These vectors are copied from the specifications and must never be edited to make a test pass:

| Vector | Test |
|---|---|
| NIP-OA preimage, message hash, tag and tag bytes (owner secret 1, agent secret 2) | `domain/attestation/attestation_test.go` |
| NIP-OA invalid vectors (`kind=1&`, `kind=01`, self-attestation, wrong element count) | `domain/attestation/attestation_test.go` |
| NIP-OA signed event example: ID and signature | `domain/nostr/nostr_test.go`, `adapters/crypto/secp256k1/secp256k1_test.go` |
| NIP-OA auth signature verification | `adapters/crypto/secp256k1/secp256k1_test.go` |
| NIP-19 npub and nsec | `domain/key/key_test.go` |
| BIP-173 valid and invalid bech32 strings | `domain/key/bech32_test.go` |

The domain's stdlib curve check is cross-checked against btcec on 256 inputs (`TestDomainCurveCheck_AgreesWithBtcec`).

## 5. Fuzz targets

Fuzz targets sit next to the code they fuzz (`FuzzParseConditions`, `FuzzParseAuthTag`, `FuzzBech32RoundTrip`, `FuzzBech32Decode`). Their seed corpora run as ordinary unit tests in `make test`. `make fuzz FUZZTIME=5m` mutates each target. Commit a crasher under the package's `testdata/fuzz/` as a new seed, together with the fix.

## 6. Naming and structure

- Name a test after the behaviour it pins: `TestSave_NeverOverwrites`, `TestCheck_ExpiredAttestationNeverReachesTheRelay`. Never after the task that produced it.
- A test named for a class ("every", "never", "any") must fail if one member of the class breaks, including the near miss.
- Table tests use a `map[string]…` or a slice with a `name`; failures print the case name.
- One behaviour per test function. Helpers call `t.Helper()`.

## 7. Checklist for review

- [ ] Right category and location (§1).
- [ ] No sleep, no wall clock, no outside network (§2).
- [ ] App tests use fakes; adapter tests use real libraries (§3).
- [ ] Specification vectors untouched (§4).
- [ ] Name states the behaviour (§6).
- [ ] Errors compared by code.
