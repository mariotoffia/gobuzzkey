---
applyTo: "adapters/**"
---

# Adapters

- Each adapter implements ports, ends with `var _ ports.X = …` assertions, and takes options as `WithXxx`. Adapters never import each other, `app` or `cmd`.
- A library is imported only in the adapter's `acl_*.go`; exported signatures use domain types (lint enforces the import, review the signatures).
- `buzzhttp` signs every request as the agent with NIP-98 over the exact URL and body it sends, sends `x-auth-tag` only for attested identities, reads at most 16 MiB, and classifies every non-2xx answer into a `shared.Code` (403 with `relay_membership_required` is `RELAY_MEMBERSHIP_REQUIRED`, 401 `RELAY_UNAUTHORIZED`, other 4xx `RELAY_REJECTED`, 5xx and network errors `RELAY_UNAVAILABLE`). Relay text placed in an error goes through `shared.Printable`. A change here is mirrored in `testutil/fakerelay`.
- `envfile` creates files with `O_EXCL` and mode 0600, never overwrites, keeps the Buzz variable names, and single-quotes `BUZZ_RELAY_URL` and `BUZZ_AUTH_TAG` in env format (ADR-0003).
- `terminal` never echoes input and reads at most one line from a pipe.
- `secp256k1` produces BIP-340 signatures and x-only keys; it must keep agreeing with the domain's curve check.
