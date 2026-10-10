# Measurement definitions

[Documentation](README.md) · [Project overview](../README.md)

Use these definitions when reviewing a run, choosing pass criteria, or comparing
tools. Counts describe logical workload execution; they are not wire captures.

## Workload accounting

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
  generator and HTTP diagnostics. Distributed results use schema 4; configured
  server telemetry uses schema 5 in either execution mode. `workload` identifies the arrival unit.
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
peaks can miss spikes. See [docs/diagnostics.md](diagnostics.md) for definitions
and limitations. Use `-diagnostics=false` or `"diagnostics": false` to disable
instrumentation when measuring its overhead.

Optional target-server telemetry scrapes selected gauges and counters during the
run, with one final observation after drain. Live/final charts show explicit gaps
and counter resets. Exporter requests stay separate from workload accounting,
and exporter failures do not alter load checks. The coordinator alone collects
metrics in distributed tests. Use `examples/telemetry.json` with the rebuilt demo,
or configure your server's exporter as described in [docs/telemetry.md](telemetry.md).
`-telemetry=false` disables this separately from generator diagnostics.

Distributed rate-mode tests assign each arrival once across a shared schedule
and divide the concurrency cap into fixed worker quotas. Global percentiles
merge histogram buckets, and missing worker results fail the run without
inventing counts. Worker resource measurements remain per process. Remote
control requires authenticated HTTPS; local worker processes can use loopback
HTTP. See [docs/workers.md](workers.md) for setup and limitations.

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

## Reading a verdict

A pass means the run completed and its configured acceptance checks passed.
It does not establish that every response was fast, that the service is healthy
under all conditions, or that the generator delivered every planned arrival if
you disabled the drop check. Inspect errors, missed demand, latency, and generator
resources together.

For journeys, distinguish HTTP request thresholds from whole-journey thresholds.
An unsuccessful assertion can fail an HTTP attempt even with a 200 response.
A skipped dependent step sends no request and contributes no request latency.

The dashboard comparison is descriptive: differences are B minus A, percentage
changes use A as the baseline when nonzero, and percentage-rate differences are
percentage points. Warnings identify known changes in configuration or execution
conditions. There is no automatic statistical regression verdict.

## Result formats

| Schema | Execution | Additional measurements |
| --- | --- | --- |
| 3 | Local | Journey accounting and optional generator/HTTP diagnostics |
| 4 | Distributed | Worker status, assignments, and result coverage |
| 5 | Local or distributed | Configured target-server telemetry |

The dashboard also accepts older schema 2 run bundles. A run bundle includes
configuration, final results, and its retained timeline; CLI result JSON alone
cannot be imported as a run bundle. Import validation checks consistency but
cannot verify the original traffic. Treat imported measurements as supplied data.
See [history and portable files](dashboard.md#history-and-portable-files).

## Comparing tools fairly

Keep target behavior, machine resources, offered workload, concurrency, arrival
spacing, connection reuse, TLS, response consumption, and instrumentation
comparable. Account for generator misses and the complete response body. Repeat
runs and retain the test inputs and environment alongside results.

No published benchmark in this repository establishes that gmeter is faster or
more accurate than JMeter. The [parity program](jmeter-parity.md) tracks functional
coverage separately from performance evidence.
