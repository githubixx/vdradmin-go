#!/usr/bin/env bash
set -euo pipefail

from_ref="${1:-}"

if [[ -z "$from_ref" ]]; then
  echo "usage: $0 <from-commit-or-tag>" >&2
  exit 2
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

# Resolve and validate.
from_sha="$(git rev-parse "${from_ref}^{commit}")"
head_sha="$(git rev-parse HEAD)"

# Collect commit subjects (oldest->newest) from from_ref (inclusive).
# Exclude merge commits to keep it readable.
subjects="$(
  git log --reverse --format='%s' "$from_sha"^..HEAD |
    grep -vE '^(Merge |merge |Merge pull request )' || true
)"

emit_group() {
  local title="$1"
  local awk_filter="$2"

  echo "## $title"
  if [[ -z "$subjects" ]]; then
    echo "- (none)"
    echo
    return 0
  fi

  # awk_filter must print matching lines (already prefixed with "- ").
  local out
  out="$(echo "$subjects" | awk "$awk_filter")"
  if [[ -z "$out" ]]; then
    echo "- (none)"
  else
    echo "$out"
  fi
  echo
}

cat <<EOF
# Release Notes Preview

Range: $from_sha..$head_sha (inclusive start)
EOF

echo

# shellcheck disable=SC2016
emit_group "✨ Added" '
  /^feat(\([[:alnum:]_-]+\))?!?:/ { print "- ✨ " $0 }
' 

# shellcheck disable=SC2016
emit_group "🐛 Fixed" '
  /^fix(\([[:alnum:]_-]+\))?!?:/ { print "- 🐛 " $0 }
' 

# shellcheck disable=SC2016
emit_group "🛠️ Changed" '
  /^(chore|refactor|perf|build|ci|docs|test|style|deps)(\([[:alnum:]_-]+\))?!?:/ {
    if ($0 ~ /^chore/) icon="🧹"
    else if ($0 ~ /^refactor/) icon="♻️"
    else if ($0 ~ /^perf/) icon="⚡️"
    else if ($0 ~ /^docs/) icon="📝"
    else if ($0 ~ /^build/) icon="🏗️"
    else if ($0 ~ /^ci/) icon="🤖"
    else if ($0 ~ /^test/) icon="✅"
    else if ($0 ~ /^deps/) icon="⬆️"
    else if ($0 ~ /^style/) icon="🎨"
    else icon="🛠️"
    print "- " icon " " $0
  }
' 

# shellcheck disable=SC2016
emit_group "🧩 Other" '
  !/^feat(\([[:alnum:]_-]+\))?!?:/ &&
  !/^fix(\([[:alnum:]_-]+\))?!?:/ &&
  !/^(chore|refactor|perf|build|ci|docs|test|style|deps)(\([[:alnum:]_-]+\))?!?:/ {
    print "- 🧩 " $0
  }
' 
