# 0001 — Agents get their own key, admitted through an owner attestation

Status: accepted
Date: 2026-10-21
Deciders: gobuzzkey maintainers

## Context

An agent that should read and write on a Buzz relay needs a key the relay admits. Buzz relays normally require relay membership (NIP-43). There are three ways to give an agent access:

1. Give the agent the owner's own secret key.
2. Ask the relay operator to enroll a dedicated agent key as a relay member.
3. Give the agent a dedicated key and an owner attestation (NIP-OA). A relay running with `BUZZ_ALLOW_NIP_OA_AUTH=true` admits the agent when the attestation verifies and the owner is a member (NIP-AA).

Option 1 makes every agent message look like the owner wrote it. Tools that filter out the identity's own events, such as the `buzz_inbox` tool of the buzz-grokbot MCP server, then never see what the owner writes to the agent. It also hands a long-lived personal key to a process the owner may not control. Option 2 works on every relay but needs the operator for every agent. Option 3 is the mechanism Buzz itself uses for the agents its desktop app spawns.

## Decision

gobuzzkey implements option 3 as its primary path. `new` generates a fresh agent key, asks for the owner's key once, signs `SHA256("nostr:agent-auth:" || agent || ":" || conditions)` with it, and stores the agent identity with the tag. The owner's key is never stored.

Option 2 is supported as a secondary path: an identity file without `BUZZ_AUTH_TAG` is a member identity, and `verify` and `check` accept it. That way the same tooling still works when an operator prefers direct enrollment.

The only condition gobuzzkey writes is `created_at<` (from `-expires`). Kind clauses are not offered: an agent typically publishes several kinds, a relay ignores kind clauses at admission, and two kind clauses can never both hold.

## Consequences

- The relay must run with `BUZZ_ALLOW_NIP_OA_AUTH=true`, which is off by default in Buzz. `check` names this setting when admission fails.
- An attestation cannot be revoked once issued; it stays valid until its expiry, and it admits the agent for as long as the owner remains a member. The README recommends `-expires`. Removing the agent from its channels cuts it off from those channels immediately.
- Relay access and channel access stay separate: the owner still adds the agent's npub to each private channel.
