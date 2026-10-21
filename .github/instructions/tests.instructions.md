---
applyTo: "**/*_test.go,tests/**,testutil/**"
---

# Tests

The contract is TESTS.md; its §7 checklist is the review list. `make test` already rejects `time.Sleep` in tests.

- Unit tests use `clocktest.At` wherever a clock is taken, including `fakerelay.WithClock`; mixing real and fixed clocks makes the fake relay reject NIP-98 events.
- Only `tests/live` reaches a real relay, it is skipped under `-short` and without `GOBUZZKEY_LIVE_ENV`, and it never posts.
- `app` tests use the hand-written fakes; adapter tests use the real library; `cmd` tests call `run` with an injected environment.
- Specification vectors (NIP-OA, NIP-19, BIP-173) are never edited to make a test pass.
- Errors are compared with `shared.IsCode` or `errors.Is`, never by message text.
- A test names the behaviour it pins. No planning identifiers in names or messages.
- `testutil/fakerelay` must implement Buzz's admission rules exactly as `ARCHITECTURE.md §4` describes them; a test that needs it to be laxer is testing the wrong thing.
