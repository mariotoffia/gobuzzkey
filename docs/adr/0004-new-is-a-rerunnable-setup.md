# 0004 — `new` is a re-runnable setup, and the agent joins channels itself

Status: proposed
Date: 2026-10-22
Deciders: gobuzzkey maintainers

## Context

Setting up an agent took several commands and manual steps: mint, add the agent npub to the channel in Buzz, run `check`, then copy three values into a Grok Bot chat. People re-run setup commands, and a re-run must not mint a second agent that is not in the channel.

Buzz channels are NIP-29 groups. Anyone admitted to the relay may join an open channel with a kind-9021 request. Only a channel member may add someone to a private channel, with a kind-9000 event. The owner is usually that member, but using the owner key for it would break the rule that the owner key signs exactly one thing, the attestation (ADR-0001).

## Decision

- `new` keeps an existing identity file when it is for the same relay and its attestation has not expired, and asks for the owner key only when it mints. An identity for another relay (`IDENTITY_EXISTS`) or an expired one (`ATTESTATION_EXPIRED`) stops the run; `remove` clears it. The identity file is still never overwritten (ADR-0003).
- After the identity, `new` checks admission and, with `-channel`, joins the channel with a kind-9021 request signed by the agent. It reads the kind-39002 member list first, so a re-run publishes nothing. For a private channel it prints the agent npub to add in Buzz Desktop instead of using the owner key a second time.
- A new attestation expires after 3 years unless `-expires` says otherwise. An attestation cannot be withdrawn, so a bound keeps a leaked agent key from working forever.
- `new` ends by printing the Grok Bot prompt between ``` fences, ready to copy. It holds the agent key, so the identity store writes it, the same way `-out -` prints the identity; it is never written to a file, which would be a second copy of the key at rest. The prompt runs buzz-grokbot pinned to a reviewed commit, because that server holds the agent key; the pin moves only after the diff has been read.
- `remove` leaves the channel with a kind-9022 request signed by the agent and deletes the identity file. If the agent cannot leave, the file is kept so the command can be repeated.

## Consequences

- `new` talks to the relay, and its exit status is that of the first failing step. The identity is written before the relay is contacted, so a refusal costs nothing: fix it and run the same command again.
- The owner key is still used for exactly one signature.
- A private channel always needs one manual step in Buzz Desktop.
- The agent key is printed after every `new`, so it sits in the terminal scrollback. The copy pasted into Grok Bot has to be removed there.
