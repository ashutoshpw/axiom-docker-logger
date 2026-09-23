#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <tag> [tag...]" >&2
  exit 1
fi

if [ ! -d plugin/rootfs ]; then
  echo "plugin/rootfs is missing; run 'make clean build' first" >&2
  exit 1
fi

for tag in "$@"; do
  make PLUGIN_TAG="$tag" create
  if [ "${DRY_RUN:-0}" = "1" ]; then
    echo "DRY_RUN=1: skipping push of $tag"
  else
    make PLUGIN_TAG="$tag" push
  fi
  make PLUGIN_TAG="$tag" rm
done
