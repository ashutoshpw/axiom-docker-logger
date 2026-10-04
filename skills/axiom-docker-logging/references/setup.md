# Setup commands

## Install on the selected daemon

Choose the suffix from the Docker **server** architecture:

```sh
docker version --format '{{.Server.Os}}/{{.Server.Arch}}'
# Select amd64 or arm64 from that result, then:
plugin_ref=ashutoshpw/axiom-docker-logger:latest-arm64
# Alternative registry, once that release is verified available:
# plugin_ref=ghcr.io/ashutoshpw/axiom-docker-logger:latest-arm64
```

Examples assume `AXIOM_TOKEN` is already supplied through an authorized secret source
and `AXIOM_DATASET` is set to an existing dataset. Do not print either variable or enable
shell tracing. The commands below use placeholders through variables, not token literals.

```sh
: "${AXIOM_TOKEN:?Supply the token securely}"
: "${AXIOM_DATASET:?Choose an existing dataset}"
docker plugin install --disable "$plugin_ref"
docker plugin set "$plugin_ref" "AXIOM_TOKEN=$AXIOM_TOKEN" "AXIOM_DATASET=$AXIOM_DATASET"
docker plugin enable "$plugin_ref"
```

Installation may prompt for plugin privileges; those are not credential prompts.
Automation can use `--grant-all-permissions` only within an authorized plugin installation.
The plugin uses host networking. For a disabled existing plugin, set values and enable it.
For an enabled plugin, first coordinate its dependent containers before disabling it.
Do not reinstall an existing working plugin merely to apply a dataset override.

## Docker run

```sh
docker run --log-driver "$plugin_ref" --log-opt "axiom-dataset=$AXIOM_DATASET" IMAGE
```

Reconstruct existing workloads from their maintained run configuration or Compose file;
do not replace them with a minimal command that loses mounts, ports, or environment.

## Docker Compose

Merge only the logging section into selected services. This example is AMD64:

```yaml
services:
  app:
    image: your-existing-image
    logging:
      driver: ashutoshpw/axiom-docker-logger:latest-amd64
      options:
        axiom-dataset: your-existing-dataset
```

Use the actual installed reference or alias, including its architecture suffix. Do not
add the ingestion token to this file. Validate with `docker compose config --quiet`.
When authorized to apply, recreate the selected service:

```sh
docker compose up -d --no-deps --force-recreate app
```

This can interrupt that service. Preserve the old logging section for rollback.

## Marker verification

With permission to run a temporary container:

```sh
marker="axiom-check-$(date -u +%Y%m%dT%H%M%SZ)-$$"
docker run --rm --log-driver "$plugin_ref" --log-opt "axiom-dataset=$AXIOM_DATASET" \
  alpine:3.20 sh -c 'printf "%s\n" "$1"; printf "%s-stderr\n" "$1" >&2' sh "$marker"
printf 'Look for marker: %s\n' "$marker"
```

Search the selected Axiom dataset for those exact message values in the recent time range.
Expected fields include `_time`, `message`, `source`, `container_id`, `container_name`,
`container_image`, and container labels when present. Confirm both stdout and stderr.
Allow for network and query visibility latency; if absent after 60 seconds, diagnose
rather than claiming success or endlessly restarting containers.

## Optional daemon default

Only when the user explicitly requests a daemon-wide default, merge into existing
`/etc/docker/daemon.json` without replacing unrelated keys:

```json
{
  "log-driver": "ashutoshpw/axiom-docker-logger:latest-amd64",
  "log-opts": {"axiom-dataset": "your-existing-dataset"}
}
```

Validate the daemon configuration and coordinate Docker restart. Credentials still belong
to the plugin. This changes defaults for newly created containers, not existing ones.
