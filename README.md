# Axiom Docker Logger

Docker managed logging driver that sends container stdout/stderr to Axiom.
Supports native rootful Linux Docker Engine on AMD64 and ARM64.

[![skills.sh](https://skills.sh/b/ashutoshpw/axiom-docker-logger)](https://skills.sh/ashutoshpw/axiom-docker-logger)

## Agent setup skill

```bash
npx skills add ashutoshpw/axiom-docker-logger --list
npx skills add ashutoshpw/axiom-docker-logger --skill axiom-docker-logging
```

Ask your agent: “Configure Axiom logging for my Docker Compose app using axiom-docker-logging.”
The [skill](skills/axiom-docker-logging/SKILL.md) covers inspection, credentials, targeted
configuration changes, marker verification, and troubleshooting. It is self-contained
when installed by the skills CLI. skills.sh discovers public installations through
[CLI telemetry](https://skills.sh/docs/faq); publication of files alone does not prove a listing is visible.

## Quick start

Create an Axiom dataset and an ingestion token restricted to it. Supply `AXIOM_TOKEN`
securely in your shell and set `AXIOM_DATASET` to that existing dataset. Do not commit
credentials or enable shell tracing around credential commands.

```bash
docker version --format '{{.Server.Os}}/{{.Server.Arch}}'
# Select latest-arm64 instead for a Linux ARM64 daemon.
plugin_ref=ashutoshpw/axiom-docker-logger:latest-amd64
: "${AXIOM_TOKEN:?Supply your token securely}"
: "${AXIOM_DATASET:?Choose an existing dataset}"
docker plugin install --disable "$plugin_ref"
docker plugin set "$plugin_ref" "AXIOM_TOKEN=$AXIOM_TOKEN" "AXIOM_DATASET=$AXIOM_DATASET"
docker plugin enable "$plugin_ref"
docker run --log-driver "$plugin_ref" --log-opt "axiom-dataset=$AXIOM_DATASET" nginx
```

The installation prompt is for privileges, not credentials. The plugin uses host networking.
Token settings are visible to Docker administrators. Windows engines, rootless Docker,
and Docker Desktop are not covered by the native Linux support checks.

## Compose and configuration

```yaml
services:
  app:
    image: myapp
    logging:
      driver: ashutoshpw/axiom-docker-logger:latest-amd64
      options:
        axiom-dataset: my-app-logs
```

Use the exact installed reference or alias. The dataset must exist and be accessible to
the plugin's token. `axiom-dataset` overrides the plugin default. `axiom-token` is not
supported: credentials belong to the plugin's `AXIOM_TOKEN` setting.

Plugin settings: `AXIOM_TOKEN` (required), `AXIOM_DATASET` (default `docker-logs`), and
`AXIOM_URL` (default `https://api.axiom.co`). To change them, coordinate dependent
containers, disable the plugin, set values, and enable it again. Do not force-disable
an in-use plugin. Existing containers must be **recreated** to change their logging driver.

For [daemon-wide defaults and full setup examples](skills/axiom-docker-logging/references/setup.md),
merge the documented settings into existing daemon configuration and coordinate restart.

## Delivery and verification

Batches flush at 100 events or approximately one second. A bounded in-memory queue applies
backpressure: slow ingestion can block container logging in Docker's default blocking mode.
The driver retries transient failures three times with 1/2/4-second backoff, bounds each
request to 10 seconds, and allows 30 seconds for shutdown drain. It reports rejected counts
and never replays partially accepted batches wholesale.

There is no persistent queue or exactly-once guarantee. Host crashes, exhausted retries,
and shutdown deadlines can lose events; ambiguous network failures may cause duplicates.
Docker's optional non-blocking mode can also drop records when its buffer fills.

Use the [marker verification procedure](skills/axiom-docker-logging/references/setup.md#marker-verification)
to confirm actual delivery in Axiom. Plugin enablement and `docker logs` do not prove it:
Docker may serve logs from its local cache. An ingestion-only token cannot query Axiom.
See [troubleshooting](skills/axiom-docker-logging/references/troubleshooting.md).

## Distribution and tags

The publishing workflow targets these **managed plugin** repositories:

- Docker Hub: `ashutoshpw/axiom-docker-logger`
- GHCR: `ghcr.io/ashutoshpw/axiom-docker-logger`

| Tags | Meaning |
| --- | --- |
| `edge-amd64`, `edge-arm64` | Verified main builds |
| `X.Y.Z-amd64`, `X.Y.Z-arm64` | Immutable version tags |
| `latest-amd64`, `latest-arm64` | Latest verified stable release |
| `edge`, `X.Y.Z`, `latest` | AMD64 aliases retained for compatibility |

Prereleases receive version tags but do not advance latest. Install these with
`docker plugin install`, not `docker pull`. Architecture selection is explicit.
Registry support and anonymous installation must pass before release aliases are promoted.
Check the publishing workflow for availability; adding workflow code does not publish a release.

## Development and publishing

Go 1.24+ and a native Linux Docker daemon are required:

```bash
make test           # race tests and vet
make build          # native rootfs under plugin/rootfs
make smoke          # real plugin delivery to a local receiver; no Axiom credentials
```

The smoke check creates temporary plugins/containers and removes only its own resources.
Run it on a development/CI daemon. `make clean` removes only generated local rootfs files.

GitHub Actions tests on native AMD64 and ARM64. Push main for edge publication or a
`vX.Y.Z` tag for a release. Configure `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repository
secrets. GHCR uses `GITHUB_TOKEN` with `packages: write`; make its package public in GitHub
package settings before anonymous verification can pass. Docker Hub must also be public.

Candidates are uploaded under unique SHA/run tags and installed on fresh native runners
from both registries. All verification jobs must pass before digest-preserving promotion.
A GHCR plugin-format rejection fails publication; no ordinary image is substituted.
Version tags cannot be overwritten with different content. Reruns that would change an
existing version require a new release version. Registry updates are not transactional;
a partial promotion can be retried against the same verified candidate digests.
