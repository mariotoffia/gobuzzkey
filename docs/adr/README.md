# Architecture Decision Records

An ADR records a decision with lasting consequences: what was decided, why, and what follows from it. Code and docs cite them as `ADR-NNNN`. An accepted ADR is not edited to change its decision; a new ADR supersedes it.

| ADR | Decision |
|---|---|
| [0001](0001-agent-identity-via-owner-attestation.md) | Agents get their own key, admitted through an owner attestation (NIP-OA / NIP-AA). |
| [0002](0002-stdlib-only-domain-crypto-behind-ports.md) | The domain is stdlib-only; signing sits behind ports. |
| [0003](0003-identity-file-format.md) | The identity file uses Buzz's variable names and is never overwritten. |
| [0004](0004-new-is-a-rerunnable-setup.md) | `new` is a re-runnable setup; the agent joins open channels with its own key. |

Template: copy an existing ADR, number it next, and set `Status: proposed` until it is merged.
