# Reliable logging and agent setup

## Objective
Fix idle flushing, shutdown delivery, rejected-event handling, and configuration docs.
Publish native AMD64/ARM64 managed plugins to Docker Hub and GHCR; ship a root skills/ setup skill.

## Decisions
- Bounded memory, 100-event batches, one-second flush, backpressure.
- Stop drains available FIFO records without waiting for Docker to close its writer.
- Ten-second ingest attempts, thirty-second shutdown; report losses without payloads/secrets.
- Plugin environment owns credentials; container configuration owns dataset override.
- Native architecture tags; unsuffixed tags remain AMD64 aliases.
- Verify candidate plugin installation from both registries before alias promotion.
- Guided skill; no installer script or normal container-image substitute.

## Steps
- [x] Regression tests, driver lifecycle and retry fixes.
- [ ] Native architecture builds and actual Docker delivery smoke checks.
- [ ] Candidate publication, registry verification and guarded promotion workflows.
- [x] Correct README and self-contained skills/axiom-docker-logging skill.
- [ ] Validate, commit scoped changes, record evidence and remaining external gates.

## Acceptance
Race tests and vet pass; original defects reproduced before fixing.
Docker stdout/stderr reaches a local receiver through the plugin.
Both native CI architectures and registries must pass before publication is called verified.
Skill discovery and isolated installation succeed. Live Axiom and skills.sh visibility
are separate external checks, never inferred from local success.

## Evidence
- Baseline idle and shutdown cases timed out; partial ingestion incorrectly returned success.
- Fixed baseline regressions passed under race detection.
- AMD64 Docker plugin smoke passed idle and immediate-exit stdout/stderr delivery.
- Skill validator, local discovery and isolated installation with both references passed.
- Docker Hub already has 0.1.0/latest/edge from September 23; next release is 0.2.0.
- Docker Hub Actions secrets exist; GHCR package has not yet been created.
