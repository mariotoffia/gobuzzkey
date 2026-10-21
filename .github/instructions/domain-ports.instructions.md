---
applyTo: "domain/**,ports/**"
---

# Domain and ports

The domain is stdlib-only and does no I/O; lint enforces that. Review what lint cannot see.

- Value objects are immutable and comparable. Aggregate roots (`Attestation`, `AgentIdentity`) live in `*_aggregate.go`, carry `// aggregate-root`, keep their fields unexported, are built only through validating constructors, and expose `Validate() error`.
- Parsing is strict for wire forms (lowercase hex, exact tag shape, canonical decimals) and lenient only for user input (`ParsePublicKey`, `ParseSecretKey` accept any case and bech32).
- `Conditions` keeps the original string. Nothing may reorder, deduplicate or re-render clauses before signing or verifying.
- `nostr.Serialize` escapes like serde_json and JSON.stringify (`"` `\`, short forms for `\b \t \n \f \r`, `\u00xx` for other C0 controls, nothing else). Any change must keep the NIP-OA signed-event vector passing.
- `key.SecretKey` keeps its bytes in a closure so it stays redacted even as an unexported field of another struct. It must not gain a JSON or text marshaller.
- `relay.ParseURL` accepts scheme, host and port only; Buzz's NIP-98 check has no room for a path.
- Text from outside (relay responses, file contents) that may be shown to a user goes through `shared.Printable`.
- Every error is a `*shared.Error` with the most specific existing code. A new code needs the wiring listed in `AGENTS.md`.
- Ports stay small and use domain types only. A new port method that only one adapter can implement is a smell.
- Names follow `UBIQUITOUS.md`: owner and agent, never delegator or child; attestation, never delegation.
