# gobuzzkey

gobuzzkey gives an AI agent or bot its own identity on a [Buzz](https://github.com/block/buzz) relay. It generates a key pair for the agent, has you sign an owner attestation for it with your own Buzz key (Nostr [NIP-OA](https://github.com/block/buzz/blob/main/docs/nips/NIP-OA.md)), and writes the three variables every Buzz agent reads: `BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY` and `BUZZ_AUTH_TAG`. It then signs in to the relay as the agent to prove the identity works.

## Why

A Buzz relay usually admits only its members. An agent such as a Grok Bot, a Claude or Cursor agent, or a script needs a key the relay accepts. You can give the agent your own key, but then everything it writes appears as you, and tools that skip their own messages (for example the `buzz_inbox` tool of the [buzz-grokbot](https://github.com/davidorban/buzz-grokbot) MCP server) never see the messages you write to it.

The better arrangement is the one Buzz itself uses for its agents. The agent gets its own key, and you, a relay member, sign a statement that the agent acts on your behalf. Relays that run with `BUZZ_ALLOW_NIP_OA_AUTH=true` admit the agent because you are a member (NIP-AA), and the relay operator never has to enroll it. gobuzzkey does this from the command line, with your key used once and never stored.

## Install

```sh
go install github.com/mariotoffia/gobuzzkey/cmd/gobuzzkey@latest
```

Prebuilt binaries for macOS, Linux and Windows are attached to each [release](https://github.com/mariotoffia/gobuzzkey/releases).

## Quick start

1. Copy your Buzz private key. In Buzz desktop: Settings, Profile, Private key.

2. Mint the agent identity. Paste your key at the hidden prompt.

   ```sh
   gobuzzkey new -relay wss://buzz.example.com -expires 2027-12-31
   ```

   This writes `buzz-agent.env` (mode 0600) and prints the agent's npub.

3. In Buzz, open the channel the agent should use, choose members, paste the agent npub and add it.

4. Check that the relay admits the agent and that it can read and post:

   ```sh
   gobuzzkey check -channel my-channel -send "hello from the agent"
   ```

5. Give the three variables in `buzz-agent.env` to the agent. [docs/guides/grok-bot.md](docs/guides/grok-bot.md) walks through a Grok Bot with the buzz-grokbot MCP server.

## Commands

### `gobuzzkey new`

Generates the agent key and the attestation and writes the identity.

| Flag | Default | Meaning |
|---|---|---|
| `-relay` | required | Relay URL: `ws`, `wss`, `http` or `https`, host and optional port only. Buzz serves its API at the host root, so a path is rejected. Stored as the `http(s)` base URL. |
| `-out` | `buzz-agent.env` | Destination. `-` writes the identity to stdout and the summary to stderr. An existing file is never overwritten. |
| `-format` | `env` | `env` (shell-sourceable) or `json` (one object, usable as an MCP server `env` block). Anything else is a usage error. |
| `-expires` | none | `YYYY-MM-DD` (00:00 UTC), an RFC 3339 time, or a duration such as `90d` or `36h`. Adds a `created_at<` condition, so the relay stops admitting the agent at that time. Must be in the future and no later than 2106-02-07, the last time a NIP-OA condition can express. |

The owner key is read from a hidden prompt on a terminal, from one line of piped stdin, or from `$BUZZ_OWNER_PRIVATE_KEY`. It may be an `nsec1…` or 64 hex characters.

### `gobuzzkey verify`

Checks an identity offline: that the secret key matches the attested agent, that the owner signature verifies, and whether the attestation has expired.

```sh
gobuzzkey verify -env buzz-agent.env
gobuzzkey verify -tag '["auth","79be…","created_at<1830000000","8b7d…"]' -agent npub1…
```

The second form verifies an attestation someone else issued, without any secret key.

### `gobuzzkey check`

Signs in to the relay as the agent (NIP-98, plus the attestation in the `x-auth-tag` header), lists the channels the agent can see and, with `-channel`, reads that channel. `-send` posts a message to it as the agent, carrying the auth tag so Buzz can show who authorized it. Without `-send` nothing is written. Buzz does not require channel names to be unique; when a name matches several channels, `check` lists their IDs and posts nothing.

| Flag | Default | Meaning |
|---|---|---|
| `-env` | `buzz-agent.env` | Identity file (env or JSON). |
| `-channel` | none | Channel name or ID to read. An ID always wins. |
| `-send` | none | Text to post to `-channel`. |
| `-history` | 3 | How many recent messages to read. |

## The identity file

```sh
# gobuzzkey agent identity. Secret: anyone with this file can act as the agent.
# agent npub1…
# owner npub1…
# expires 2027-12-31T00:00:00Z
BUZZ_RELAY_URL='https://buzz.example.com'
BUZZ_PRIVATE_KEY=nsec1…
BUZZ_AUTH_TAG='["auth","<owner-hex>","created_at<1830211200","<signature-hex>"]'
```

These are the variable names `buzz-cli`, `buzz-acp` and the buzz-grokbot MCP server read. The relay URL and the auth tag are single-quoted so that `source` and dotenv loaders keep the tag's double quotes. Docker's `--env-file` does not strip quotes; use `-format json` or remove them for Docker. When you paste a value into a form, leave out the outer single quotes.

An identity without `BUZZ_AUTH_TAG` is valid too. It describes an agent the relay operator enrolled as a member directly, and `verify` and `check` accept it.

## What the relay must allow

The owner key, yours, must be a member of the relay. The relay must run with `BUZZ_ALLOW_NIP_OA_AUTH=true`, which is off by default in Buzz. If it is off, `check` exits with status 5 and `RELAY_MEMBERSHIP_REQUIRED`. The operator then either enables it or adds the agent npub as a relay member, in which case the agent works without the auth tag. Relay access and channel access are separate: the agent also has to be added to each private channel it should use.

## Exit status

| Status | Meaning |
|---|---|
| 0 | Success. |
| 1 | Unclassified failure. |
| 2 | Bad command line. |
| 3 | Invalid input: a key, attestation, condition, relay URL or identity file, or the destination exists. |
| 4 | Trust failure: the signature does not verify, owner and agent are the same key, or the attestation expired. |
| 5 | The relay refused the agent: not admitted (`RELAY_MEMBERSHIP_REQUIRED`) or NIP-98 rejected (`RELAY_UNAUTHORIZED`). |
| 6 | The relay rejected a request or is unavailable. |
| 7 | The channel is not visible to the agent, or its name matches several channels. |
| 130 | Interrupted (Ctrl-C), for example at the owner-key prompt. |

## Security

Your owner key is used for one signature and never written anywhere. The agent key is written only to the destination you choose, with mode 0600, and gobuzzkey never prints it. A secret key prints as `nsec1…(redacted)` under every `fmt` verb, also inside other structs, and a lint rule confines revealing one to the identity store. Text that comes from the relay, such as channel names and error messages, has control characters escaped before it reaches your terminal.

An attestation cannot be withdrawn once issued. The relay keeps admitting the agent until the attestation expires or you stop being a relay member. Removing the agent from a channel cuts its access to that channel at once, but not to the relay. So prefer `-expires`, and ask the relay operator to block the agent's key if it must be shut out before then.

## Development

The repository follows the same layout and gates as [gobridge](https://github.com/mariotoffia/gobridge): hexagonal layers, DDD bounded contexts, machine-enforced architecture and two commands that decide when work is done.

```sh
make install   # pinned lint tools
make lint      # every static check; reports under reports/
make test      # unit tests with the race detector
```

Start with [AGENTS.md](AGENTS.md), then [ARCHITECTURE.md](ARCHITECTURE.md), [DDD.md](DDD.md) and [UBIQUITOUS.md](UBIQUITOUS.md). [DEVELOPMENT.md](DEVELOPMENT.md) covers local setup and [RELEASE.md](RELEASE.md) how releases are cut.

## License

Apache License 2.0. See [LICENSE](LICENSE).
