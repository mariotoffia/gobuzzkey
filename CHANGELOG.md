# Changelog

All notable changes to gobuzzkey are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). See
[RELEASE.md](RELEASE.md) for how releases are cut.

## [Unreleased]

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
