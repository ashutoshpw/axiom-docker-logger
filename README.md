# Axiom Docker Logger

Docker logging driver for Axiom.co

## Quick Start

One-time setup:

```bash
docker plugin install ashutoshpw/axiom-docker-logger:latest \
  AXIOM_TOKEN=xaat-xxxxx \
  AXIOM_DATASET=docker-logs
```

Run containers with the driver:

```bash
docker run --log-driver=ashutoshpw/axiom-docker-logger:latest myapp
```

Or in `docker-compose.yml`:

```yaml
services:
  app:
    image: myapp
    logging:
      driver: ashutoshpw/axiom-docker-logger:latest
      options:
        axiom-dataset: my-app-logs
```

## Installation

Install the plugin:

```bash
docker plugin install ashutoshpw/axiom-docker-logger:latest
```

## Configuration

Configure credentials (prompted during install, or set after):

```bash
docker plugin set ashutoshpw/axiom-docker-logger:latest AXIOM_TOKEN=xaat-xxxxx
docker plugin set ashutoshpw/axiom-docker-logger:latest AXIOM_DATASET=my-logs
```

Enable if not already enabled:

```bash
docker plugin enable ashutoshpw/axiom-docker-logger:latest
```

## Tags

| Tag | Description |
| --- | --- |
| `latest` | Latest stable release, updated when a `v*` tag is pushed |
| `X.Y.Z` | Immutable release, e.g. `1.2.3` |
| `edge` | Latest build from `main`, may be unstable |

## Usage

Use it with your containers:

```bash
docker run --log-driver=ashutoshpw/axiom-docker-logger:latest nginx
```

### Set as Default Daemon-Wide

To make this the default logging driver for all containers, configure it in `/etc/docker/daemon.json`:

```json
{
  "log-driver": "ashutoshpw/axiom-docker-logger:latest",
  "log-opts": {
    "axiom-token": "xaat-xxxxx",
    "axiom-dataset": "docker-logs"
  }
}
```

Then restart Docker:

```bash
sudo systemctl restart docker
```
