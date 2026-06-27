#!/usr/bin/env bash
set -euo pipefail

allowlist_file="${GOVULNCHECK_ALLOWLIST:-.govulncheck.allowlist}"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

set +e
GOTOOLCHAIN=auto go run golang.org/x/vuln/cmd/govulncheck@latest ./... >"$tmp" 2>&1
status=$?
set -e

cat "$tmp"

if [[ "$status" -eq 0 ]]; then
  exit 0
fi

mapfile -t found < <(grep -Eo 'GO-[0-9]{4}-[0-9]+' "$tmp" | sort -u)
if [[ "${#found[@]}" -eq 0 ]]; then
  echo "govulncheck failed without reporting GO vulnerability IDs" >&2
  exit "$status"
fi

declare -A allowed=()
today="$(date -u +%F)"

if [[ -f "$allowlist_file" ]]; then
  while IFS='|' read -r id expires reason; do
    id="${id//[[:space:]]/}"
    expires="${expires#expires }"
    expires="${expires//[[:space:]]/}"
    [[ -z "$id" || "${id:0:1}" == "#" ]] && continue
    if [[ -z "$expires" || "$expires" < "$today" ]]; then
      echo "expired or invalid govulncheck allowlist entry for $id (expires: ${expires:-missing})" >&2
      exit 1
    fi
    allowed["$id"]="${reason:-allowlisted}"
  done <"$allowlist_file"
fi

unallowed=()
for id in "${found[@]}"; do
  if [[ -z "${allowed[$id]+set}" ]]; then
    unallowed+=("$id")
  fi
done

if [[ "${#unallowed[@]}" -gt 0 ]]; then
  echo "govulncheck found non-allowlisted vulnerabilities: ${unallowed[*]}" >&2
  exit 1
fi

echo "govulncheck found only reviewed allowlisted vulnerabilities: ${found[*]}"
