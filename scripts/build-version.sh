#!/bin/sh
set -eu

run_number="${1:-${GITHUB_RUN_NUMBER:-}}"
if ! printf '%s' "$run_number" | grep -Eq '^[1-9][0-9]*$'; then
  echo "build number must be a positive integer" >&2
  exit 1
fi

repository_root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
series="$(tr -d '[:space:]' < "$repository_root/VERSION")"
if ! printf '%s' "$series" | grep -Eq '^[0-9]+\.[0-9]+$'; then
  echo "VERSION must contain a major.minor number" >&2
  exit 1
fi

printf '%s.%s\n' "$series" "$run_number"
