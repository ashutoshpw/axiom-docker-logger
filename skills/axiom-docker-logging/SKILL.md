---
name: axiom-docker-logging
description: Configure Docker container stdout and stderr delivery to Axiom using the axiom-docker-logger managed plugin. Use for Docker run or Compose logging setup, dataset routing, and troubleshooting missing container logs on Linux AMD64 or ARM64 hosts.
---

# Axiom Docker logging

Configure the user's selected Docker services and verify delivery. This is a Docker
managed logging plugin installed on the Docker daemon host; it is not a sidecar,
application SDK, Kubernetes collector, or ordinary container image.

## Inspect first

- Inspect the active Docker context and **server** OS/architecture, not the CLI host.
  Use `docker version --format '{{.Server.Os}}/{{.Server.Arch}}'` and `docker context show`.
- Support native rootful Linux Docker Engine on amd64 or arm64. For Docker Desktop,
  rootless engines, Windows engines, or other architectures, explain the unverified
  environment rather than claiming support or silently changing the user's context.
- Find the target Compose file/services or existing container's logging configuration.
  Inspect plugin names and enabled state with `docker plugin ls`. Do not dump plugin
  environment values: they contain the Axiom token.
- Determine whether the user wants a configuration edit or actual installation/recreation.
  Reuse authorization already given; do not turn configuration work into an unrequested
  daemon-wide rollout, credential rotation, or application restart.
- Obtain the existing dataset name and authorized token source. Prefer a token restricted
  to ingestion into that dataset. Do not ask the user to paste secrets into chat or commit
  them. The dataset must already exist; creating it is a separate Axiom operation.

## Configure

Read [setup.md](references/setup.md) for concrete install, Docker run, and Compose
commands. Use the daemon's architecture to choose `latest-amd64` or `latest-arm64`,
or pin a known published version with the same suffix. Prefer Docker Hub; use GHCR
when requested and the selected artifact is available. Do not silently switch registries
on authentication errors.

Credentials belong to the plugin's `AXIOM_TOKEN` environment. The supported container
option is `axiom-dataset`; `axiom-token` is unsupported. Keep token literals out of
shell history, command transcripts, Compose, and Git. Turn off shell tracing when using
an existing secret environment variable. Plugin settings remain visible to Docker admins.

Preserve unrelated Compose settings. An already-correct, enabled plugin can be reused.
A plugin in use cannot normally be disabled: credential changes require coordinating
its dependent containers. Do not force-disable or remove it to bypass this constraint.
Existing containers need recreation to adopt a new driver; restarting alone is insufficient.
Apply only the selected services' changes with the user's authorized operational scope.

## Verify and report

- Check the target container's logging driver and dataset option without printing secrets.
- Emit a unique marker from a short-lived container using the same plugin and dataset.
- Confirm the exact marker in Axiom, checking message, source and container metadata.
  An ingestion-only token cannot query data. Use an authorized query integration or ask
  the user to confirm the marker in the Axiom UI; do not request broader credentials
  merely to finish a local setup task.
- Report separately: configuration written, plugin enabled, containers recreated,
  and marker confirmed in Axiom. Neither `docker logs` output nor plugin enablement
  proves remote delivery. Docker may satisfy `docker logs` using its local cache.

For failures, read [troubleshooting.md](references/troubleshooting.md). Preserve the
user's previous logging configuration for rollback; rollback also requires recreation.
