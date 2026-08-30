#!/bin/sh
set -eu

repository_root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
fixture="$(mktemp -d "${TMPDIR:-/tmp}/cairnops-release-notes-test.XXXXXX")"
trap 'rm -rf "$fixture"' EXIT HUP INT TERM

git -C "$fixture" init -q -b main
git -C "$fixture" config user.name "CairnOps tests"
git -C "$fixture" config user.email "tests@cairnops.invalid"
mkdir -p "$fixture/.github"
cp "$repository_root/.github/initial-release-notes.md" "$fixture/.github/initial-release-notes.md"

printf '%s\n' "foundation" > "$fixture/example.txt"
git -C "$fixture" add example.txt .github/initial-release-notes.md
git -C "$fixture" commit -q -m "feat: initialize fixture"

initial_output="$(
  cd "$fixture"
  GITHUB_REPOSITORY="example/cairnops" \
    sh "$repository_root/scripts/release-notes.sh" 0.1.1 "" HEAD
)"

printf '%s' "$initial_output" | grep -Fq "Première publication versionnée de CairnOps."
printf '%s' "$initial_output" | grep -Fq 'ghcr.io/m0okz/cairnops-server:0.1.1'
printf '%s' "$initial_output" | grep -Fq 'https://github.com/example/cairnops/tree/v0.1.1'

git -C "$fixture" tag v0.1.1

printf '%s\n' "feature" >> "$fixture/example.txt"
git -C "$fixture" commit -qam "feat(web): afficher le détail des incidents"

printf '%s\n' "fix" >> "$fixture/example.txt"
git -C "$fixture" commit -qam "fix: corriger le compteur"

printf '%s\n' "internal" >> "$fixture/example.txt"
git -C "$fixture" commit -qam "ci: ajuster le cache"

printf '%s\n' "breaking" >> "$fixture/example.txt"
git -C "$fixture" commit -qam "feat(api)!: modifier le contrat des incidents"

release_output="$(
  cd "$fixture"
  GITHUB_REPOSITORY="example/cairnops" \
    sh "$repository_root/scripts/release-notes.sh" 0.1.2 v0.1.1 HEAD
)"

printf '%s' "$release_output" | grep -Fq "## Incompatibilités"
printf '%s' "$release_output" | grep -Fq -- "- Modifier le contrat des incidents"
printf '%s' "$release_output" | grep -Fq "## Nouveautés"
printf '%s' "$release_output" | grep -Fq -- "- Afficher le détail des incidents"
printf '%s' "$release_output" | grep -Fq "## Corrections"
printf '%s' "$release_output" | grep -Fq -- "- Corriger le compteur"
printf '%s' "$release_output" | grep -Fq 'https://github.com/example/cairnops/compare/v0.1.1...v0.1.2'

if printf '%s' "$release_output" | grep -Fq "ajuster le cache"; then
  echo "internal changes must not appear in release notes" >&2
  exit 1
fi

printf '%s\n' "release notes tests passed"
