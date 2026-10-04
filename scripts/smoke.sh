#!/usr/bin/env bash
# Run on an isolated, native Linux Docker host. Optional argument: registry plugin ref.
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d)
alias="axiom-smoke-$(date +%s)-$$"
receiver_pid=
created=0
cleanup() {
  docker rm -f "$alias-idle" "$alias-stop" >/dev/null 2>&1 || true
  if [[ "$created" == 1 ]]; then docker plugin disable "$alias" >/dev/null 2>&1 || true; docker plugin rm "$alias" >/dev/null 2>&1 || true; fi
  if [[ -n "$receiver_pid" ]]; then kill "$receiver_pid" 2>/dev/null || true; wait "$receiver_pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
"${GO:-go}" build -o "$work/receiver" ./internal/smokereceiver
"$work/receiver" -ready "$work/ready" >"$work/receiver.log" 2>&1 &
receiver_pid=$!
for _ in {1..100}; do [[ -s "$work/ready" ]] && break; sleep .1; done
url=$(cat "$work/ready")
if [[ $# -gt 0 ]]; then
  # An anonymous pull proves users can install the public artifact.
  mkdir "$work/docker"
  DOCKER_CONFIG="$work/docker" docker plugin install --disable --grant-all-permissions --alias "$alias" "$1"
else
  docker plugin create "$alias" "${PLUGIN_DIR:-plugin}"
fi
created=1
docker plugin set "$alias" AXIOM_TOKEN=xaat-smoke AXIOM_DATASET=smoke "AXIOM_URL=$url"
docker plugin enable "$alias"
docker run -d --name "$alias-idle" --log-driver "$alias" alpine:3.20 sh -c 'echo idle; sleep 30' >/dev/null
for _ in {1..50}; do
  if curl -fsS "$url/verify?key=stdout:idle" >/dev/null 2>&1; then break; fi
  sleep .1
done
curl -fsS "$url/verify?key=stdout:idle" >/dev/null
docker run --name "$alias-stop" --log-driver "$alias" --log-opt axiom-dataset=smoke alpine:3.20 sh -c 'echo final-out; echo final-err >&2' >/dev/null
curl -fsS "$url/verify?key=stdout:final-out&key=stderr:final-err" >/dev/null
echo "PASS: idle flush, shutdown stdout/stderr, metadata and dataset routing ($alias)"
