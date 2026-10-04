# Troubleshooting

| Symptom | Check and response |
| --- | --- |
| Plugin exits during enablement | Confirm AXIOM_TOKEN was configured while disabled. Install does not prompt for it. Never dump full plugin inspection output. |
| Exec format error | Compare Docker server architecture to the tag; unsuffixed tags are AMD64. |
| Plugin is in use | Identify dependent containers and coordinate changes. Do not force-disable or remove shared plugins. |
| Missing dataset / HTTP 403 | Verify dataset existence, selected dataset override, token ingestion permissions, and Axiom region/endpoint. |
| HTTP 401 | Correct the plugin credential through the authorized secret source. Changing application env or axiom-token log options does not configure the plugin. |
| HTTP 429 or 5xx | Check Axiom availability and limits. The driver retries three times, then reports losses; it has no persistent spool. |
| Driver changed but old behavior remains | Recreate the selected container; restarting retains its original logging configuration. |
| docker logs works but Axiom has no data | Docker's local log cache is separate from remote delivery. Confirm an exact marker in Axiom. |
| GHCR pull denied | Check package visibility and published tag availability. Packages must be public for anonymous installation. |
| Shutdown takes time | Shutdown allows up to 30 seconds to drain and deliver; request attempts have a 10-second deadline. |

On systemd Linux hosts inspect recent Docker daemon logs for the plugin's messages,
filtering to the affected plugin/container. Share only sanitized excerpts. Do not paste
full environment, authorization headers, or application log payloads into diagnostics.

The driver flushes at 100 events or roughly one second, subject to network backpressure.
It uses a bounded queue; slow ingestion can block application logging in Docker's default
blocking mode. Docker's non-blocking logging mode trades that blocking for possible drops.
Host crashes, exhausted retries, or shutdown timeout can lose data. Ambiguous request
failures can cause duplicate delivery on retry. Partially accepted batches are reported
and never replayed wholesale.
