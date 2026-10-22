# gobuzzkey

gobuzzkey gives an AI agent or bot its own identity on a [Buzz](https://github.com/block/buzz) relay. It generates a key pair for the agent, has you sign an owner attestation for it with your own Buzz key (Nostr [NIP-OA](https://github.com/block/buzz/blob/main/docs/nips/NIP-OA.md)), and writes the three variables every Buzz agent reads: `BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY` and `BUZZ_AUTH_TAG`. It then signs in to the relay as the agent to prove the identity works.

## Why

A Buzz relay usually admits only its members. An agent such as a Grok Bot, a Claude or Cursor agent, or a script needs a key the relay accepts. You can give the agent your own key, but then everything it writes appears as you, and tools that skip their own messages (for example the `buzz_inbox` tool of the [buzz-grokbot](https://github.com/davidorban/buzz-grokbot) MCP server) never see the messages you write to it.

The better arrangement is the one Buzz itself uses for its agents. The agent gets its own key, and you, a relay member, sign a statement that the agent acts on your behalf. Relays that run with `BUZZ_ALLOW_NIP_OA_AUTH=true` admit the agent because you are a member (NIP-AA), and the relay operator never has to enroll it. gobuzzkey does this from the command line, with your key used once and never stored.

## Install

```sh
go install github.com/mariotoffia/gobuzzkey/cmd/gobuzzkey@latest
```

Prebuilt binaries for macOS, Linux and Windows are attached to each [release](https://github.com/mariotoffia/gobuzzkey/releases). From a checkout, `make install-cli` builds the current source and installs it the same way.

## Quick start

1. Copy your Buzz private key. In Buzz Desktop: Settings, Profile, Private key.

2. Run one command. Paste your key at the hidden prompt.

   ```sh
   gobuzzkey new -relay wss://buzz.example.com -channel my-channel
   ```

   ```text
   ok    identity  created buzz-agent.env (mode 0600)
                   agent    npub1…
                   owner    npub1…  (must be a member of the relay)
                   relay    https://buzz.example.com
                   expires  2029-10-22T09:14:03Z
   ok    grok      wrote buzz-agent.grok.txt (mode 0600, holds the agent key)
   ok    relay     https://buzz.example.com admits the agent
   ok    channel   joined my-channel (5f0c…)
   ok    check     read 3 recent message(s) in my-channel
   ```

   It creates the agent identity, checks that the relay admits the agent, joins the channel and reads it. When a step fails it says what to do: what the relay admin has to change, or that a private channel needs the agent added in Buzz Desktop (Members, add the printed npub). Fix that and run the same command again. A second run keeps the identity, does not ask for your key and skips what is already done.

3. Give the identity to the agent. For a Grok Bot, paste the whole of `buzz-agent.grok.txt` into a chat with the bot ([docs/guides/grok-bot.md](docs/guides/grok-bot.md)). Other agents read the three variables in `buzz-agent.env`.

To start over, `gobuzzkey remove -channel my-channel` takes the agent out of the channel and deletes both files.

## Commands

### `gobuzzkey new`

Sets up an agent in steps, printing `ok` or `FAIL` for each, and stops at the first failure:

1. **identity**: generates the agent key and the attestation and writes the identity, or keeps the identity already at `-out` when it is for the same relay and has not expired. An identity for another relay or an expired one stops the run; `remove` clears it.
2. **grok**: writes the Grok Bot setup message next to the identity (`buzz-agent.grok.txt`, mode 0600). It holds the agent key and is rewritten on every run.
3. **relay**: signs in as the agent to prove the relay admits it.
4. **channel**: with `-channel`, joins the channel as the agent. Buzz lets anyone join an open channel; a private channel needs a member to add the agent, so `new` prints the npub to add in Buzz Desktop instead.
5. **check**: reads the channel's recent messages.

| Flag | Default | Meaning |
|---|---|---|
| `-relay` | required | Relay URL: `ws`, `wss`, `http` or `https`, host and optional port only. Buzz serves its API at the host root, so a path is rejected. Stored as the `http(s)` base URL. |
| `-channel` | none | Channel name or ID to join and read. |
| `-out` | `buzz-agent.env` | Identity file; kept when it exists, never overwritten. `-` writes the identity to stdout and the steps to stderr, and writes no Grok Bot message. |
| `-format` | `env` | `env` (shell-sourceable) or `json` (one object, usable as an MCP server `env` block). Anything else is a usage error. |
| `-expires` | `3y` | Expiry of a new identity: `YYYY-MM-DD` (00:00 UTC), an RFC 3339 time, or a duration such as `3y`, `90d` or `36h`. Adds a `created_at<` condition, so the relay stops admitting the agent at that time. Must be in the future and no later than 2106-02-07, the last time a NIP-OA condition can express. Ignored when the identity is kept. |

The owner key is needed only when a new identity is created. It is read from a hidden prompt on a terminal, from one line of piped stdin, or from `$BUZZ_OWNER_PRIVATE_KEY`, and may be an `nsec1…` or 64 hex characters.

### `gobuzzkey remove`

Undoes `new` and is safe to run again: with `-channel` the agent leaves that channel, then the identity file and its Grok Bot message are deleted. If the agent cannot leave (the relay is down, say), both files are kept so you can try again; run without `-channel` to delete them anyway. Remove the `buzz` MCP server from the Grok Bot yourself: it holds a copy of the agent key.

| Flag | Default | Meaning |
|---|---|---|
| `-env` | `buzz-agent.env` | Identity file to delete. |
| `-channel` | none | Channel name or ID the agent leaves first. |

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

A relay that admits everyone (Buzz without `BUZZ_REQUIRE_RELAY_MEMBERSHIP=true`) needs nothing. A members-only relay admits the agent when the operator does one of these:

- runs the relay with `BUZZ_ALLOW_NIP_OA_AUTH=true`, which is off by default in Buzz. The owner key, yours, must be a relay member;
- adds the agent npub as a relay member, in which case the agent works without the auth tag.

Until then `new` and `check` exit with status 5 and `RELAY_MEMBERSHIP_REQUIRED`, and `new` prints both options with the npubs filled in. Relay access and channel access are separate: the agent also has to be a member of each channel it should use. It can join an open channel itself; a private one needs a member to add it.

## Exit status

| Status | Meaning |
|---|---|
| 0 | Success. |
| 1 | Unclassified failure. |
| 2 | Bad command line. |
| 3 | Invalid input: a key, attestation, condition, relay URL or identity file, or the identity file is for another relay. |
| 4 | Trust failure: the signature does not verify, owner and agent are the same key, or the attestation expired. |
| 5 | The relay refused the agent: not admitted (`RELAY_MEMBERSHIP_REQUIRED`) or NIP-98 rejected (`RELAY_UNAUTHORIZED`). |
| 6 | The relay rejected a request or is unavailable. |
| 7 | The channel is not visible to the agent, or its name matches several channels. |
| 130 | Interrupted (Ctrl-C), for example at the owner-key prompt. |

## Security

Your owner key is used for one signature and never written anywhere. The agent key is written only to the identity file and its Grok Bot message, both with mode 0600, and gobuzzkey never prints it. A secret key prints as `nsec1…(redacted)` under every `fmt` verb, also inside other structs, and a lint rule confines revealing one to the identity store. Text that comes from the relay, such as channel names and error messages, has control characters escaped before it reaches your terminal.

An attestation cannot be withdrawn once issued. The relay keeps admitting the agent until the attestation expires or you stop being a relay member. Removing the agent from a channel cuts its access to that channel at once, but not to the relay. That is why `new` sets a 3-year expiry unless you pass `-expires`. Ask the relay operator to block the agent's key if it must be shut out before then.

## Development

The repository follows the same layout and gates as [gobridge](https://github.com/mariotoffia/gobridge): hexagonal layers, DDD bounded contexts, machine-enforced architecture and two commands that decide when work is done.

```sh
make install       # pinned lint tools
make install-cli   # build gobuzzkey from this checkout into $(go env GOPATH)/bin
make lint          # every static check; reports under reports/
make test          # unit tests with the race detector
```

Start with [AGENTS.md](AGENTS.md), then [ARCHITECTURE.md](ARCHITECTURE.md), [DDD.md](DDD.md) and [UBIQUITOUS.md](UBIQUITOUS.md). [DEVELOPMENT.md](DEVELOPMENT.md) covers local setup and [RELEASE.md](RELEASE.md) how releases are cut.

## License

Apache License 2.0. See [LICENSE](LICENSE).
