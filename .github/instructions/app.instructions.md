---
applyTo: "app/**"
---

# Use cases

- Use cases depend on ports and domain only, receive every collaborator in their constructor, and hold no global state.
- `Minter.Mint` checks the relay URL, the expiry and `IdentityStore.Writable` before calling `SecretPrompt.OwnerSecret`. A new precondition that does not need the owner key goes before the prompt too.
- The owner secret is used for exactly one `Signer.Sign` call and is never stored, returned or put into an error.
- A freshly signed attestation is verified before it is stored.
- `Inspector.Open` derives the agent key from the secret key and verifies the signature against it; it must never trust a public key read from a file.
- `Checker.Check` rejects an expired attestation before the first relay call and reports `Admitted` only after the relay answered.
- Tests use `app/fakes_test.go`, never an adapter.
