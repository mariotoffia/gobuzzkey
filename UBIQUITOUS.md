# gobuzzkey — Ubiquitous Language

## Overview

Authoritative glossary. One term per row. If a concept appears in code, flags, output or docs, it uses this name and this meaning. Grouped by bounded context (see [DDD.md](DDD.md)).

## Roles

| Term | Meaning |
|---|---|
| **Owner** | The person whose Buzz key vouches for an agent. Must be a relay member. In the code: `owner`, `Owner()`. Never "delegator" or "parent". |
| **Agent** | The process (bot, LLM agent, script) that acts on the relay under its own key. In the code: `agent`. Never "bot key" or "child". |
| **Relay** | The Buzz server, addressed by its base URL. |
| **Relay operator** | Whoever runs the relay and controls `BUZZ_ALLOW_NIP_OA_AUTH` and membership. |
| **Member** | A key the relay admits on its own (NIP-43 relay membership). |

## Keys (`domain/key`)

| Term | Meaning |
|---|---|
| **PublicKey** | BIP-340 x-only secp256k1 public key, 32 bytes. Shown as `npub1…`, sent as 64 lowercase hex. |
| **SecretKey** | secp256k1 scalar, 32 bytes. Read as `nsec1…` or hex. Prints redacted under every `fmt` verb, also as a field of another struct. |
| **Signature** | 64-byte BIP-340 Schnorr signature, sent as 128 lowercase hex. |
| **npub / nsec** | NIP-19 bech32 forms of a public and a secret key. |
| **Wire hex** | Lowercase hex as it appears in tags and events. User input may be upper case; wire hex may not. |

## Attestation (`domain/attestation`)

| Term | Meaning |
|---|---|
| **Attestation** | An owner's signed statement that an agent key may act on the owner's behalf (NIP-OA). The aggregate `Attestation` pairs it with the agent key it was issued for. Never "delegation" (that is NIP-26, which reassigns authorship; NIP-OA does not). |
| **AuthTag** | The wire form of an attestation: `["auth", <owner-hex>, <conditions>, <sig-hex>]`. Carried in `BUZZ_AUTH_TAG` and the `x-auth-tag` header. |
| **Conditions** | The constraint string inside an auth tag: empty, or clauses joined by `&`. Signed verbatim. |
| **Clause** | One condition: `kind=<n>`, `created_at<<t>` or `created_at><t>`. |
| **Preimage** | `"nostr:agent-auth:" + agent-hex + ":" + conditions`. |
| **Message** | `SHA256(Preimage)`, the bytes the owner signs. |
| **Expiry** | The smallest `created_at<` bound: the first second the attestation no longer covers. Set with `-expires`. |
| **Admissible** | A relay evaluating the attestation now would accept its time conditions (`Attestation.AdmittedAt`). Kind clauses are ignored at admission. |
| **Self-attestation** | Owner and agent are the same key. Invalid. |

## Nostr (`domain/nostr`)

| Term | Meaning |
|---|---|
| **Event** | A signed Nostr event as it travels on the wire. |
| **Unsigned** | An event whose ID is computed but which is not yet signed. Its ID is the message the author signs. |
| **Canonical serialization** | `[0,pubkey,created_at,kind,tags,content]` escaped the way serde_json and JSON.stringify do it. Its SHA-256 is the event ID. |
| **Filter** | A NIP-01 query filter sent to `/query`. |
| **HTTP auth event** | Kind 27235 event (NIP-98) proving who sent an HTTP request: tags `u`, `method`, `nonce`, `payload`. |
| **Kind 9** | A channel message (NIP-29 group chat). |
| **Kind 39000** | Channel metadata (NIP-29 group metadata): `d` = channel ID, `name`, `about`. |

## Relay (`domain/relay`)

| Term | Meaning |
|---|---|
| **Relay URL** | The relay's base URL: scheme, host and optional port, normalized to `http(s)`. `wss://` becomes `https://`. No path: Buzz checks NIP-98 against `scheme://host/route`. |
| **Channel** | A Buzz channel, which is a NIP-29 group. Identified by its ID (the `d` tag, a UUID on Buzz) and named by `name`. Referenced on the command line by ID or name. |
| **Admitted** | The relay answered the agent's first authenticated query. |
| **Printable** | Text from outside made terminal-safe: control characters escaped, length capped (`shared.Printable`). |
| **WriteResult** | The relay's verdict on a published event: event ID, accepted, message. |

## Identity (`domain/identity`)

| Term | Meaning |
|---|---|
| **AgentIdentity** | Relay URL + agent secret key + agent public key + (usually) attestation. What `new` writes and `verify`/`check` read. |
| **Attested identity** | An identity with an attestation; the relay admits it through the owner. |
| **Member identity** | An identity without an attestation; the agent itself must be a member. |
| **Identity file** | The stored identity: env lines or a JSON object with `BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY`, `BUZZ_AUTH_TAG`. Never overwritten. |

## Use cases (`app`)

| Term | Meaning |
|---|---|
| **Mint** | Create an attested identity: generate the agent key, sign, store. Command `new`. |
| **Open** | Read an identity file and verify it (key pairing, signature). |
| **Inspect / verify** | Report on an attestation as a relay would judge it now. Command `verify`. |
| **Check** | Prove an identity against a live relay. Command `check`. |

## Error codes (`domain/shared`)

| Code | Meaning | Exit status |
|---|---|---|
| `INVALID_KEY` | A key failed to parse or is not a valid secp256k1 key. | 3 |
| `INVALID_CONDITIONS` | A conditions string breaks the grammar, or `-expires` is not in the future. | 3 |
| `INVALID_TAG` | An auth tag is not a well-formed 4-element NIP-OA tag. | 3 |
| `INVALID_RELAY_URL` | The relay URL is not ws, wss, http or https, or carries a query, fragment or user info. | 3 |
| `INVALID_EVENT` | An event or filter could not be built or decoded, or message text is not valid UTF-8. | 3 |
| `IDENTITY_INVALID` | An identity file is missing variables or holds an invalid value. | 3 |
| `IDENTITY_EXISTS` | The identity destination already exists. | 3 |
| `SECRET_UNAVAILABLE` | The owner key could not be read or parsed. Exit status 130 instead when reading was interrupted (Ctrl-C). | 3 |
| `SELF_ATTESTATION` | Owner and agent are the same key. | 4 |
| `SIGNATURE_INVALID` | The owner signature does not verify for this agent. | 4 |
| `ATTESTATION_EXPIRED` | The attestation's expiry has passed. | 4 |
| `RELAY_MEMBERSHIP_REQUIRED` | The relay did not admit the agent: no membership, NIP-OA admission off, owner not a member, or attestation expired. | 5 |
| `RELAY_UNAUTHORIZED` | The relay rejected the NIP-98 signature (often clock skew). | 5 |
| `RELAY_REJECTED` | Any other client error, or a write the relay did not accept. | 6 |
| `RELAY_UNAVAILABLE` | The relay could not be reached or failed with a 5xx. | 6 |
| `CHANNEL_NOT_VISIBLE` | The named channel is not visible to the agent. | 7 |
| `CHANNEL_AMBIGUOUS` | More than one visible channel has the given name; pass the ID. | 7 |

## Environment variables

| Variable | Read by | Meaning |
|---|---|---|
| `BUZZ_RELAY_URL` | identity file, Buzz agents | Relay base URL. |
| `BUZZ_PRIVATE_KEY` | identity file, Buzz agents | Agent secret key (`nsec1…`). |
| `BUZZ_AUTH_TAG` | identity file, Buzz agents | Auth tag JSON. Absent for a member identity. |
| `BUZZ_OWNER_PRIVATE_KEY` | `gobuzzkey new` | Owner secret key, instead of the prompt. |
| `GOBUZZKEY_LIVE_ENV`, `GOBUZZKEY_LIVE_CHANNEL` | `tests/live` | Identity file and optional channel for the live relay test. |
