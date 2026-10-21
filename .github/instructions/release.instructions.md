---
applyTo: ".github/workflows/**,Makefile,CHANGELOG.md,go.mod"
---

# Release and CI

The contract is RELEASE.md.

- Workflow actions are pinned by full commit SHA with the version in a comment. Jobs default to `contents: read`; only the publish job gets `contents: write`.
- The Release workflow keeps its order: tag policy (protected, non-forced creation, `vX.Y.Z`), validation on the tagged commit (on `main`, changelog heading, `make lint test`), build (`make dist`), then publish after re-checking that the tag still points at the validated commit.
- Tool versions in `Makefile` (`make install`) are the ones CI uses; bump them in one place.
- A user-visible change has an entry under `## [Unreleased]`. A released section is never rewritten.
- `go.mod` keeps `go 1.27.0` as the minimum and a `toolchain` line; a dependency bump keeps `make check` green.
