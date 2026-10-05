# gmeter

An HTTP load and stress-testing tool that makes offered traffic and missed
demand visible. The Go CLI and local browser dashboard support single requests
and multi-step application journeys, with portable reports for both.
Development stages are tracked in [ROADMAP.md](ROADMAP.md).

## Quick start

Requires Go 1.24 or newer. There are no third-party Go dependencies.

```sh
make build

# Terminal 1: start the local target.
./bin/gmeter-demo

# Terminal 2: validate and run a small test.
./bin/gmeter validate -config examples/smoke.json
./bin/gmeter run -config examples/smoke.json
```

Open the HTML file printed at the end of the run. JSON and HTML files are saved
under `results/` by default, with unique timestamped filenames.

For the interactive app, run `./bin/gmeter ui` and open the printed URL
(normally http://127.0.0.1:8790). Build and validate tests, watch live charts,
save scenarios, compare run history, and import/export configurations and run
bundles. Saved workspace files live in `.gmeter/` by default. The demo target
still runs separately. See [docs/dashboard.md](docs/dashboard.md) for details.

```sh
# Fixed concurrent users: each user waits for its response before continuing.
./bin/gmeter run -url http://127.0.0.1:8787/work -vus 4 -duration 5s

# Fixed arrivals: target 100 requests/second independently of response speed.
./bin/gmeter run -url http://127.0.0.1:8787/work -rate 100 -duration 5s \
  -max-in-flight 32 -p95 150ms

# Stress phases: baseline, overload, then recovery.
./bin/gmeter run -config examples/stress.json \
  -out results/stress.json -html results/stress.html

# Login, keep the session cookie, extract a token, and verify a checkout.
./bin/gmeter run -config examples/journey.json

# A 3:1 scenario mix with repeatable arrival jitter.
./bin/gmeter run -config examples/mix.json

# Divide the fleet rate across two authenticated workers.
# Set GMETER_WORKER_TOKEN on each process first; see docs/workers.md.
./bin/gmeter run -config examples/smoke.json \
  -workers http://127.0.0.1:8801,http://127.0.0.1:8802
```

The stress example deliberately overloads the demo server and normally exits
with code **1** because HTTP 503 responses exceed its error threshold. The
recovery row should show the server returning to normal. Thresholds currently
apply to the overall run; phase-specific acceptance checks are a later feature.

## Scenario format

JSON files are strict: unknown fields, invalid values, and trailing documents
are rejected before sending traffic. `gmeter validate` performs no requests.
Command-line flags override the corresponding scenario fields.

```json
{
  "name": "API stress test",
  "url": "http://127.0.0.1:8787/work",
  "method": "POST",
  "headers": { "Content-Type": "application/json" },
  "body": "{\"item\":\"example\"}",
  "mode": "rate",
  "max_in_flight": 64,
  "timeout": "1s",
  "stages": [
    { "name": "Baseline", "duration": "2s", "rate": 20 },
    { "name": "Overload", "duration": "3s", "rate": 300 },
    { "name": "Recovery", "duration": "2s", "rate": 20 }
  ],
  "thresholds": {
    "p95": "150ms",
    "max_error_percent": 1,
    "max_drop_percent": 1
  }
}
```

| Field | Default | Meaning |
| --- | --- | --- |
| `url` | required | Absolute HTTP or HTTPS URL |
| `name` | `HTTP test` | Label in reports |
| `method` | `GET` | HTTP method |
| `headers` | empty | Request headers; `Host` is supported |
| `body` | empty | Literal request body, up to 1 MiB |
| `mode` | `users` | `users` or `rate` |
| `duration` | `10s` | Load window; ignored when stages are present |
| `vus` | `10` | Concurrent users in users mode |
| `rate` | `0` | Integer arrivals/second in rate mode; use this or stages |
| `max_in_flight` | `100` | Maximum concurrent workload arrivals in rate mode |
| `timeout` | `5s` | Per-request timeout including connection and full response body |
| `think_time` | `0s` | Pause between each user's requests or journeys; users mode only |
| `max_response_bytes` | `10485760` | Response-body read cap; exceeding it fails the attempt |
| `diagnostics` | `true` | Generator resource sampling and HTTP transport timing; `false` disables both |
| `workers` | empty | Worker control origins for distributed rate-mode tests; empty runs locally |
| `success_statuses` | all `2xx` | Optional explicit list of successful status codes |
| `stages` | empty | Sequential constant-rate phases; rate `0` creates a pause |
| `thresholds.p95` | disabled | Maximum p95 full-response latency |
| `thresholds.max_error_percent` | `0` | Maximum percentage of completed requests that failed |
| `thresholds.max_drop_percent` | `0` | Maximum percentage of scheduled arrivals that were dropped |
| `thresholds.min_started_rps` | disabled | Minimum dispatched requests per second over the load window |

For journeys, see [docs/scenarios.md](docs/scenarios.md) for step templates,
cookie behavior, response assertions, weighted mixes, data rows, and thresholds.
The configured `rate` and concurrency cap apply to journeys when `scenarios`
is present. Request throughput is reported separately.

Setting an error or drop threshold to JSON `null` disables that check. No
completed requests, or an interrupted run, always fails. Use `run -help` for
available flags. `-out ''` and `-html ''` disable their respective artifacts.

## Measurement contract

- **Users mode** is a closed workload: response speed and think time determine
  traffic. A slower target naturally reduces arrivals in this mode.
- **Rate mode** uses a monotonic clock. With `arrival_distribution: "constant"`, each
  phase starts with an immediate arrival, then repeats every `1/rate` seconds.
  A phase schedules `ceil(duration × rate)` arrivals; adjacent phases can have
  arrivals close together at their boundary. Rates are steps, not smooth ramps.
- `arrival_distribution: "jitter"` places one arrival uniformly within each rate
  interval using `seed` (default 1). It preserves the planned arrival count and
  clips the last partial interval to the phase window. This is stratified
  jitter, not Poisson traffic. It is supported only in rate mode.
- If the concurrency cap is full, an arrival is **dropped for capacity**. If the
  scheduler falls behind by a full arrival interval, old arrivals are **dropped
  for scheduler delay**. There is no backlog or catch-up burst. A goroutine that
  has already been dispatched may still start late; `start_delay` exposes that.
- `scheduled = started + dropped_capacity + dropped_scheduler`. At the end of a
  drained run, `started = completed = succeeded + failed`. Dropped demand is
  separate from response errors and is never included in latency quantiles.
- With explicit scenarios, the above scheduling equation applies to `journeys`.
  Top-level request counts contain logical step attempts; no HTTP attempts are
  invented for a missed journey or a skipped dependent step. Report schema 3
  retains schema 2's journey, scenario, and step results and adds optional
  generator and HTTP diagnostics. `workload` identifies the arrival unit.
  `target_rate` is in that unit; `target_rps` is zero for journey workloads.
- A **started** request is a dispatched logical HTTP attempt. Go's transport may
  transparently retry certain idempotent attempts on broken reused connections;
  wire-level request counts can therefore differ. Redirects are not followed.
- **Latency** starts in the request goroutine and ends after reading the body or
  encountering an error. It includes failed attempts. **Scheduled latency** also
  includes delay from the intended arrival time to request start. These are
  client-observed measurements, not server processing times.
- Quantiles use fixed-memory logarithmic histograms. They are nearest-rank
  upper-bucket estimates with at most 1% bucket error plus 1 ns rounding; p95
  acceptance checks can be conservative near the limit. Min/max are exact;
  means use floating-point accumulation.
- **Started req/s** divides all dispatched attempts by the load window. Response
  drain time is reported separately. This is not a rate of completed responses
  within the load window. Phase results belong to the dispatch phase.
- A journey can continue its remaining steps during drain, up to
  `journey_timeout`. Those HTTP attempts are included in average request
  throughput, even when started after the admission window ends. All steps in
  a journey belong to its admission phase. Per-step latency excludes response
  verification; journey latency includes verification, extraction, and pauses.
- Planned completion stops new work and drains existing requests until their
  per-request timeouts. Ctrl+C or SIGTERM cancels active requests, saves partial
  results, and exits with code 130. Future arrivals are excluded from partial
  runs. Unvisited phases appear with a zero load duration.
- Bodies stream into a discard buffer; samples and response content are not
  retained in reports. Body assertions and JSON extraction capture at most
  1 MiB temporarily per active request. Histograms, error categories, and HTTP
  status counts are bounded. Request and journey aggregates together require
  roughly 200 KiB per phase, plus about 100 KiB per scenario and per step,
  overall metrics, and the configured concurrency. Enabled HTTP timing adds
  roughly 224 KiB to each request collector that observes transport events
  (overall, phase, and step); it adds none to journey-only or unvisited collectors.

Diagnostics show process CPU, Go memory, goroutines, GC, connection reuse,
protocol counts, and seven HTTP transport intervals. Sampling runs at start,
about once per second, and after drain. CPU and resident-peak measurements are
available on macOS and Linux; other platforms retain Go runtime measurements
and explicitly mark OS measurements unavailable. Intervals overlap, and sampled
peaks can miss spikes. See [docs/diagnostics.md](docs/diagnostics.md) for definitions
and limitations. Use `-diagnostics=false` or `"diagnostics": false` to disable
instrumentation when measuring its overhead.

Distributed rate-mode tests assign each arrival once across a shared schedule
and divide the concurrency cap into fixed worker quotas. Global percentiles
merge histogram buckets, and missing worker results fail the run without
inventing counts. Worker resource measurements remain per process. Remote
control requires authenticated HTTPS; local worker processes can use loopback
HTTP. See [docs/workers.md](docs/workers.md) for setup and limitations.

## HTTP behavior

Connections are pooled and reused. HTTPS verifies certificates and negotiates
HTTP/2 when the target supports it; cleartext targets use HTTP/1.1. Proxy settings
follow Go's standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment
variables. Automatic response decompression is disabled. Content-Length and
Transfer-Encoding are managed by the transport. Explicit scenarios have
isolated cookie jars; legacy single-request tests retain their original
behavior without a jar. Browser JavaScript is not executed.

Reports omit request headers and bodies but include the target URL. Use
authentication headers instead of putting credentials in a URL. Response-size
failures count the first byte over the configured limit and close that body,
which can prevent connection reuse.

## Exit codes and CI

| Code | Meaning |
| --- | --- |
| `0` | Run completed and all acceptance checks passed |
| `1` | An acceptance check failed |
| `2` | Invalid input or report output failure |
| `130` | Run was interrupted; partial reports were attempted |

Run the built binary in CI; `go run` wraps the executable and does not preserve
every exit code. Reports are generated even when acceptance checks fail.

```sh
make check
make build
./bin/gmeter run -config examples/smoke.json -quiet \
  -out results/ci.json -html results/ci.html
```

## Development

```text
cmd/gmeter/        Signal-aware CLI entry point
cmd/gmeter-demo/   Local server with bounded work capacity
internal/config/  Strict JSON input and validation
internal/correlation/ Variable templates and JSON Pointer lookup
internal/engine/  Scheduling, concurrency, HTTP execution, and thresholds
internal/metrics/ Bounded histograms and aggregate accounting
internal/diagnostics/ Generator process resource sampling
internal/distributed/ Worker control, leases, and fleet coordination
internal/report/  JSON and self-contained HTML output
internal/cli/     Commands, overrides, and exit codes
internal/dashboard/ Embedded browser app, local API, and persistent workspace
examples/         Local smoke, stress/recovery, POST, journeys, mixes, and workers
```

The current version is `0.5.0-dev`, with generator diagnostics, HTTP timing, and
distributed rate-mode workers implemented in milestone 4. Target-server telemetry,
automated regression checks, and a fair JMeter benchmark remain ahead.
Its efficiency and measurement accuracy have not yet been established against
JMeter.
