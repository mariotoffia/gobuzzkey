# 0003 — The identity file uses Buzz's variable names and is never overwritten

Status: accepted
Date: 2026-10-21
Deciders: gobuzzkey maintainers

## Context

The identity has to reach very different consumers: `buzz-cli` and `buzz-acp` read `BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY` and `BUZZ_AUTH_TAG` from the environment; MCP servers such as buzz-grokbot take an `env` object in their configuration; agent platforms such as Grok Bot take variables pasted into a chat. The auth tag is JSON, so it contains double quotes and commas.

## Decision

- The variable names are exactly Buzz's: `BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY`, `BUZZ_AUTH_TAG`.
- Two formats. `env`, the default: `KEY=VALUE` lines with the relay URL and the auth tag in single quotes, so that a shell's `source` and dotenv loaders keep the tag's double quotes and no value is ever shell syntax, plus comment lines with the agent and owner npubs and the expiry. The nsec is bech32 (`[a-z0-9]`) and needs no quoting. `json`: one object of strings, usable directly as an MCP server's `env` block.
- `BUZZ_RELAY_URL` is stored in its normalized `http(s)` form, which is what the REST endpoints and the NIP-98 `u` tag need, and what Buzz clients also accept.
- The private key is stored as `nsec1…`.
- Files are created with `O_EXCL` and mode 0600 and never overwritten. `new` checks the destination before asking for the owner's key. `-out -` writes the identity to stdout for piping into a secret manager, with the human-readable summary on stderr.
- Reading accepts both formats (detected by a leading `{`), `export ` prefixes, and single or double quotes.

## Consequences

- Docker's `--env-file` does not strip quotes, so it would pass the quotes along. The README says to use `-format json` or strip the quotes for Docker.
- Losing an identity file means minting a new one. That is intended: an agent key has no value beyond the agent it was minted for.
