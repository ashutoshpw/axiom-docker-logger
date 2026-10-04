#!/usr/bin/env bash
# Publish an unconfigured managed plugin, preserving the Docker plugin media type.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ $# == 1 ]] || { echo 'usage: publish.sh <registry/repository:candidate-tag>' >&2; exit 1; }
ref=$1
[[ -d "${PLUGIN_DIR:-plugin}/rootfs" ]] || { echo 'Build rootfs first' >&2; exit 1; }
docker plugin create "$ref" "${PLUGIN_DIR:-plugin}"
trap 'docker plugin rm "$ref" >/dev/null' EXIT
docker plugin push "$ref"
