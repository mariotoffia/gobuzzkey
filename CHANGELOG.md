# Changelog

All notable changes to gobuzzkey are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). See
[RELEASE.md](RELEASE.md) for how releases are cut.

## [Unreleased]

### Added

- `gobuzzkey new -channel <name>` joins the channel as the agent and reads it,
  so one command sets up an agent. A private channel cannot be joined by the
  agent itself; `new` prints the npub to add in Buzz Desktop.
- `new` writes `buzz-agent.grok.txt` next to the identity: the whole message to
  paste into a Grok Bot chat, with the three variables filled in and the
  polling routine. It holds the agent key and has mode 0600. The MCP server
  it installs is pinned to a reviewed buzz-grokbot commit.
- When the relay refuses the agent, `new` prints what the relay admin has to
  change, with the owner and agent npubs filled in.
- `gobuzzkey remove [-channel <name>]` takes the agent out of the channel and
  deletes the identity file and its Grok Bot message.
- `-expires` accepts years, such as `5y`.
- The owner-key prompt, and the error when the key is missing or invalid, say
  where to find it: Buzz Desktop, Settings, Profile, Private key.
- `make install-cli` builds the checkout and installs `gobuzzkey` into
  `$(go env GOPATH)/bin`.

### Changed

- `new` can be run again. It keeps an existing identity file for the same
  relay instead of failing with `IDENTITY_EXISTS`, asks for your key only when
  it creates a new identity, and skips a channel the agent already belongs to.
  An identity for another relay still stops it; run `remove` first.
- `new` now signs in to the relay after writing the identity. It exits with
  status 5 when the relay refuses the agent (before, only `check` did), and
  with status 7 when the channel is private or not visible.
- New identities expire after 3 years unless you pass `-expires`. Before, an
  identity without `-expires` never expired.

## [0.1.0] - 2026-10-22

### Added

- `gobuzzkey new` generates an agent key, signs a NIP-OA owner attestation for
  it with the owner's key (hidden prompt, piped stdin or
  `BUZZ_OWNER_PRIVATE_KEY`) and writes `BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY` and
  `BUZZ_AUTH_TAG` as an env file or JSON (`-format`), to a file created with
  mode 0600 or to stdout (`-out -`). `-expires` takes a date, an RFC 3339 time
  or a duration such as `90d` and adds a `created_at<` condition. An existing
  file is never overwritten, and the owner key is not requested when the
  destination exists.
- `gobuzzkey verify` checks an identity file offline (the secret key matches
  the attested agent, the owner signature verifies, the attestation has not
  expired), or a bare auth tag for a given agent with `-tag` and `-agent`.
- `gobuzzkey check` signs in to the relay as the agent with NIP-98 and the
  attestation in `x-auth-tag`, lists the channels the agent can see, reads one
  with `-channel` and posts to it with `-send`. Posted messages carry the auth
  tag. A channel name shared by several channels is refused with their IDs
  instead of guessed.
- Distinct exit statuses for invalid input (3), trust failures (4), relay
  admission refusals (5), other relay errors (6), an invisible or ambiguous
  channel (7) and an interrupt (130), each with a hint for the next step.
- Text from the relay (channel names, error bodies) is shown with control
  characters escaped, so a relay member cannot inject terminal escape
  sequences.
- Identity files without `BUZZ_AUTH_TAG` are accepted by `verify` and `check`,
  for agents the relay operator enrolled as members directly.
