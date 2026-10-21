#!/usr/bin/env bash
#
# lint-domain-purity.sh — prove the domain layer's whole import closure is the
# Go standard library plus this module's own domain packages.
#
# .go-arch-lint.yml and .golangci.yml already forbid outward imports, but both
# are configuration a change can relax. This check reads the compiled import
# graph instead, so a relaxed rule still fails here (ARCHITECTURE.md §2).
#
# Usage: scripts/lint-domain-purity.sh   # exit 0 clean, 1 on violations

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
readonly MODULE="$(go list -m)"

violations="$(go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./domain/... |
	grep -v "^${MODULE}/domain/" || true)"

if [ -n "$violations" ]; then
	echo "domain purity: FAIL (domain/ depends on non-stdlib or outer-layer packages)" >&2
	printf '  %s\n' $violations >&2
	echo "" >&2
	echo "Fix: move the dependency behind a port in ports/ and implement it in adapters/." >&2
	exit 1
fi
echo "domain purity: OK (domain/ imports only the standard library and domain/)"
