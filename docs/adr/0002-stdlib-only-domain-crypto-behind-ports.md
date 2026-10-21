# 0002 — The domain is stdlib-only; signing sits behind ports

Status: accepted
Date: 2026-10-21
Deciders: gobuzzkey maintainers

## Context

The core of gobuzzkey is cryptographic: NIP-OA attestations and Nostr events are BIP-340 Schnorr signatures over secp256k1. The Go standard library has no secp256k1 implementation. Putting a curve library into the domain would make it the one third-party dependency every layer carries, and would tie the domain's types to that library's types.

gobridge, which this repository follows, keeps its inner ring stdlib-only and checks that by machine.

## Decision

- `domain/` imports only the standard library. `scripts/lint-domain-purity.sh` checks the compiled import graph, and `.go-arch-lint.yml` and depguard check the declared one.
- Signing, verification and key generation are ports: `ports.Signer`, `ports.Verifier`, `ports.KeyGenerator`. `adapters/crypto/secp256k1` implements all three with `github.com/btcsuite/btcd/btcec/v2`, imported only from `acl_btcec.go`. Exported signatures use domain types only.
- The domain still validates keys completely with `math/big`. An x-only public key must be below the field prime and its `x³ + 7` a quadratic residue (Euler's criterion), so it lies on the curve. A secret key must be in `[1, n-1]`. A test cross-checks the domain's curve check against btcec on 256 inputs.
- NIP-19 bech32 is implemented in the domain (BIP-173, about 100 lines) rather than imported, and pinned by the BIP-173 and NIP-19 vectors.

## Consequences

- `AgentIdentity` cannot check by itself that an agent public key belongs to its secret key, because that needs scalar multiplication. `app.Inspector.Open` derives the public key through `ports.Signer` and verifies the owner signature against it, so a mismatched file fails as `SIGNATURE_INVALID`.
- Replacing the curve library touches one adapter file.
- `app` tests use a deterministic fake signer instead of real cryptography, which keeps them independent of the adapter layer.
