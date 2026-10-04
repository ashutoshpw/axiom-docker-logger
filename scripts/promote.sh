#!/usr/bin/env bash
set -euo pipefail
[[ $# == 3 ]] || { echo 'usage: promote.sh <source-ref> <destination-ref> <immutable|mutable>' >&2; exit 1; }
source_ref=$1
destination=$2
mode=$3
source_digest=$(crane digest "$source_ref")
error_file=$(mktemp)
trap 'rm -f "$error_file"' EXIT
if current=$(crane digest "$destination" 2>"$error_file"); then
  if [[ "$current" == "$source_digest" ]]; then exit 0; fi
  if [[ "$mode" == immutable ]]; then echo "Refusing to overwrite $destination" >&2; exit 1; fi
elif ! grep -Eq 'MANIFEST_UNKNOWN|NAME_UNKNOWN|404' "$error_file"; then
  cat "$error_file" >&2; exit 1
fi
crane copy "$source_ref" "$destination"
[[ $(crane digest "$destination") == "$source_digest" ]]
echo "$destination @ $source_digest" >> "${GITHUB_STEP_SUMMARY:-/dev/stdout}"
