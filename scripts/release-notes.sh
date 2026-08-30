#!/bin/sh
set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 3 ]; then
  echo "usage: $0 <version> [previous-ref] [current-ref]" >&2
  exit 1
fi

version="$1"
previous_ref="${2:-}"
current_ref="${3:-HEAD}"

if ! printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "version must use the major.minor.build format" >&2
  exit 1
fi

repository_root="$(git rev-parse --show-toplevel)"
git rev-parse --verify "${current_ref}^{commit}" >/dev/null

if [ -n "$previous_ref" ]; then
  git rev-parse --verify "${previous_ref}^{commit}" >/dev/null
fi

repository="${GITHUB_REPOSITORY:-M0okz/cairnops}"
server_url="${GITHUB_SERVER_URL:-https://github.com}"
release_tag="v${version}"

notes_dir="$(mktemp -d "${TMPDIR:-/tmp}/cairnops-release-notes.XXXXXX")"
trap 'rm -rf "$notes_dir"' EXIT HUP INT TERM

features="$notes_dir/features"
fixes="$notes_dir/fixes"
performance="$notes_dir/performance"
security="$notes_dir/security"
documentation="$notes_dir/documentation"
improvements="$notes_dir/improvements"
breaking="$notes_dir/breaking"

touch "$features" "$fixes" "$performance" "$security" "$documentation" "$improvements" "$breaking"

capitalize() {
  awk '{ first = substr($0, 1, 1); rest = substr($0, 2); printf "%s%s", toupper(first), rest }'
}

append_change() {
  destination="$1"
  message="$2"
  polished="$(printf '%s' "$message" | capitalize)"
  printf '%s\n' "- $polished" >> "$destination"
}

if [ -z "$previous_ref" ]; then
  initial_notes="$repository_root/.github/initial-release-notes.md"
  if [ ! -f "$initial_notes" ]; then
    echo "initial release notes are missing: $initial_notes" >&2
    exit 1
  fi
  cat "$initial_notes"
  printf '\n'
else
  range="${previous_ref}..${current_ref}"
  tab="$(printf '\t')"

  git log --no-merges --format='%H%x09%s' "$range" |
    while IFS="$tab" read -r commit subject; do
      [ -n "$subject" ] || continue

      type="$(printf '%s' "$subject" | sed -E 's/^([[:alpha:]][[:alnum:]_-]*)(\([^)]*\))?!?: .*/\1/')"
      message="$(printf '%s' "$subject" | sed -E 's/^[[:alpha:]][[:alnum:]_-]*(\([^)]*\))?!?:[[:space:]]*//')"

      if printf '%s' "$subject" | grep -Eq '^[[:alpha:]][[:alnum:]_-]*(\([^)]*\))?!:' ||
        git show -s --format='%B' "$commit" | grep -Eq '^BREAKING[ -]CHANGE:'; then
        append_change "$breaking" "$message"
        continue
      fi

      case "$type" in
        feat)
          append_change "$features" "$message"
          ;;
        fix)
          append_change "$fixes" "$message"
          ;;
        perf)
          append_change "$performance" "$message"
          ;;
        security|sec)
          append_change "$security" "$message"
          ;;
        docs)
          append_change "$documentation" "$message"
          ;;
        build|chore|ci|refactor|style|test|tests)
          ;;
        *)
          append_change "$improvements" "$message"
          ;;
      esac
    done

  rendered_change=false

  render_section() {
    title="$1"
    source_file="$2"
    if [ -s "$source_file" ]; then
      printf '## %s\n\n' "$title"
      cat "$source_file"
      printf '\n'
      rendered_change=true
    fi
  }

  render_section "Incompatibilités" "$breaking"
  render_section "Sécurité" "$security"
  render_section "Nouveautés" "$features"
  render_section "Corrections" "$fixes"
  render_section "Performances" "$performance"
  render_section "Documentation" "$documentation"
  render_section "Autres améliorations" "$improvements"

  if [ "$rendered_change" = false ]; then
    printf '%s\n\n' "## Maintenance"
    printf '%s\n\n' "- Cette version contient uniquement des changements internes."
  fi
fi

printf '%s\n\n' "## Mise à jour"
printf '%s\n' "- **Sauvegarde** : sauvegardez PostgreSQL et le volume \`cairnops-secrets\` avant la mise à jour."
printf '%s\n' "- **Migrations** : le serveur les applique au démarrage avant d’accepter le trafic HTTP."

if [ -s "$breaking" ]; then
  printf '%s\n\n' "- **Incompatibilités** : consultez la section dédiée avant le déploiement."
else
  printf '%s\n\n' "- **Incompatibilités** : aucune n’est déclarée dans les changements consignés."
fi

printf '%s\n\n' "## Images publiées"
printf '%s\n' "| Composant | Image immuable |"
printf '%s\n' "| --- | --- |"
printf '%s\n' "| Serveur | \`ghcr.io/m0okz/cairnops-server:${version}\` |"
printf '%s\n' "| Worker | \`ghcr.io/m0okz/cairnops-worker:${version}\` |"
printf '%s\n\n' "| Relais Push | \`ghcr.io/m0okz/cairnops-relay:${version}\` |"

if [ -n "$previous_ref" ]; then
  printf '[Comparer avec %s](%s/%s/compare/%s...%s)\n' \
    "$previous_ref" "$server_url" "$repository" "$previous_ref" "$release_tag"
else
  printf '[Consulter le code de cette version](%s/%s/tree/%s)\n' \
    "$server_url" "$repository" "$release_tag"
fi
