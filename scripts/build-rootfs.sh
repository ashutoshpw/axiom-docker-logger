#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
arch=${ARCH:-$(docker version --format '{{.Server.Arch}}')}
case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1;; esac
out=${PLUGIN_DIR:-plugin}
image="axiom-build-${arch}:local"
docker build --platform "linux/$arch" -t "$image" .
container=$(docker create "$image" /bin/true)
trap 'docker rm "$container" >/dev/null' EXIT
mkdir -p "$out/rootfs"
docker export "$container" | tar -x -C "$out/rootfs"
if [[ "$out" != plugin ]]; then cp plugin/config.json "$out/config.json"; fi
