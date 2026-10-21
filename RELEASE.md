# Releasing gobuzzkey

## Overview

gobuzzkey is a single Go module, `github.com/mariotoffia/gobuzzkey`, released as semantic-version tags `vMAJOR.MINOR.PATCH` on `main`. A tag makes the version installable with `go install …@vX.Y.Z` and triggers the Release workflow, which builds binaries for five platforms and publishes a GitHub release with the matching `CHANGELOG.md` section as its notes.

## Policy

1. **One tag per release, from `main`.** No release branches, no prerelease or build-metadata tags.
2. **Never move or delete a tag.** The Go module proxy caches a version forever. A broken release is fixed by a new patch release.
3. **The changelog comes first.** A tag is only pushed when `CHANGELOG.md` has a `## [X.Y.Z] - YYYY-MM-DD` heading for it; the release notes are that section.
4. **Gates before tags.** `make check` (build, lint, test) passes on the exact commit being tagged.
5. **Semantic versioning.** Before v1.0.0, a minor bump may break the command line, the identity file format or the exported Go API; every such change is listed under **Changed** or **Removed** in the changelog. From v1.0.0, breaking changes need a major bump.

The identity file format (`BUZZ_RELAY_URL`, `BUZZ_PRIVATE_KEY`, `BUZZ_AUTH_TAG`) and the exit statuses are public contracts: agents and scripts depend on them.

## Required GitHub tag ruleset

The Release workflow refuses to run unless GitHub reports the tag push as a protected, non-forced creation (`created=true`, `deleted=false`, `forced=false`, `ref_protected=true`). Create this ruleset once, under Settings, Rules, Rulesets, New tag ruleset:

- target pattern `v*`;
- restrict creations, updates and deletions;
- bypass only for the maintainers who release;
- block force pushes.

Without the ruleset, a tag push fails at the first job and nothing is published.

## Changelog

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Every user-visible change adds a line under `## [Unreleased]` in the same PR, in one of **Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**, **Security**. Write for someone who uses the tool, not for a reviewer: what changed, and what they have to do about it.

## Release procedure

1. On `main`, move the `## [Unreleased]` entries under a new `## [X.Y.Z] - YYYY-MM-DD` heading, leave an empty `## [Unreleased]` above it, and merge that change.
2. Run the gates and see what would happen:

   ```bash
   make release VERSION=vX.Y.Z
   ```

   It checks a clean tree, `main` in sync with `origin/main`, that the tag does not exist yet and that the changelog heading exists, then runs `make check`. It stops before tagging.

3. Tag and push:

   ```bash
   make release VERSION=vX.Y.Z CONFIRM=1
   ```

4. The Release workflow (`.github/workflows/release.yml`) then:
   - verifies the tag event is a protected creation;
   - re-runs `make lint` and `make test` on the tagged commit;
   - builds `gobuzzkey_vX.Y.Z_<os>_<arch>` archives for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64 and windows/amd64, plus `checksums.txt` (`make dist`);
   - re-checks that the tag still points at the validated commit, then creates the GitHub release with the changelog section as notes.

5. Confirm the module proxy has it:

   ```bash
   GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go install github.com/mariotoffia/gobuzzkey/cmd/gobuzzkey@vX.Y.Z
   gobuzzkey version
   ```

## Version reporting

`gobuzzkey version` reports the module version from the build information Go embeds. `go install …@vX.Y.Z` and the release binaries (built from a tagged checkout) report `vX.Y.Z`; a local `make build` reports `(devel)` or a pseudo-version.

## Fixing a bad release

Do not delete the tag or the GitHub release. Fix forward: add the fix under **Fixed**, cut `vX.Y.(Z+1)`, and mark the broken release's notes with a line pointing at the fix. If the broken version must never be installed, add a `retract` directive for it to `go.mod` in the fix release.
