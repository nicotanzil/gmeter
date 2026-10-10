# Application journeys

[Documentation](README.md) · [Project overview](../README.md)

A scenario is an ordered sequence of HTTP steps. One scheduled arrival starts
one journey through the chosen scenario. A successful journey completes every
step and pause. The first failed step stops the journey; subsequent steps are
recorded as skipped and send no requests.

Try `examples/journey.json` against `gmeter-demo`. It logs in with two different
data rows, captures a token, keeps the session cookie, reads a profile, and
checks out. The default run schedules **20 journeys**, resulting in **60 HTTP
attempts** when all three steps succeed.

In the dashboard, load **Authenticated checkout journeys** to edit the same
configuration on the [Journey canvas](dashboard.md#edit-a-journey-on-the-flow-canvas).
The canvas supports requests, pauses, checks, extraction, variables, data rows,
weights, and per-scenario pass criteria, with Advanced JSON available alongside it.

```sh
make build
# Run in another terminal:
./bin/gmeter-demo
./bin/gmeter validate -config examples/journey.json
./bin/gmeter run -config examples/journey.json
```

## Scenario fields

| Field | Meaning |
| --- | --- |
| `name` | Required, unique scenario name |
| `weight` | Positive integer, default 1; share of scheduled journey arrivals |
| `variables` | Scenario variables overriding top-level `variables` |
| `data` | Inline rows of string values; each row must have identical keys |
| `steps` | Ordered list of 1–50 request steps |
| `thresholds` | Acceptance limits for this scenario's journeys |

At most 20 scenarios and 100 total steps are supported. The complete JSON
configuration is capped at 1 MiB. The top-level URL is the base URL for step
resolution. Top-level headers are inherited and can be overridden by a step;
top-level method/body are for legacy single-request tests, so define methods
and bodies on each step when using scenarios.

## Variables and data

Variable names use ASCII letters, digits, and underscores and cannot start
with a digit. Names are limited to 128 bytes. Each journey starts with fresh
variables, resolved in this order:

1. Top-level `variables`.
2. Scenario `variables`.
3. The selected `data` row.
4. Reserved engine values: `user_id`, `iteration`, and `journey_id`.
5. Extractions from successful steps, available to later steps and same-step assertions.

The reserved names cannot be overwritten by configuration or extraction.
`user_id` is stable for a virtual user in users mode; `iteration` starts at 1
and increments for that user. In rate mode each arrival has its own user ID and
iteration is always 1. `journey_id` is the one-based global arrival ID.

Data rows cycle by the scenario's scheduled arrival ordinal. Dropped arrivals
consume ordinals too; this keeps data selection deterministic in rate mode.
Users-mode assignment follows global admission order, which depends on thread
scheduling. Variables reset between journeys; cookies persist for a user.

Templates are compiled and checked for undefined variables before a run starts.
They expand only once: a value containing `{{...}}` remains literal data.

| Placeholder | Behavior |
| --- | --- |
| `{{token}}` | Literal substitution, suitable for a bearer header |
| `{{username\|url}}` | Query encoding (`+` for spaces) |
| `{{item_id\|path}}` | Escaping for one URL path segment |
| `{{username\|json}}` | A complete JSON-quoted string, including quotes |

Example request body: `{"username":{{username|json}}}`. Do not add quotes around
a `json` placeholder. All data values are strings. JSON number/boolean
extractions retain their textual representation; use a literal placeholder if
you intend to insert a number or boolean without quotes.

Expanded URLs, bodies, and total headers are each capped at 1 MiB. Total live
variable value bytes are capped at 1 MiB when correlation is verified. Reports
omit variable values, data rows, authorization headers, and response bodies.

## Steps and verification

Each step supports `name`, `url`, `method`, `headers`, `body`,
`success_statuses`, `extract`, `assertions`, and `pause`. The default method is
GET. Statuses inherit the top-level list, defaulting to 2xx. Relative URLs are
resolved using standard URL reference rules; a leading `/` starts at the
origin root. Absolute URLs must match the base URL's scheme, hostname, and
effective port. Expanded URLs are checked again before transmission.

```json
{
  "name": "Login",
  "url": "/login",
  "method": "POST",
  "headers": { "Content-Type": "application/json" },
  "body": "{\"username\":{{username|json}}}",
  "extract": {
    "token": { "json_pointer": "/token" },
    "session": { "header": "X-Demo-Session" }
  },
  "assertions": [
    { "json_pointer": "/user/id", "equals": "{{username}}" },
    { "header": "X-Demo-Session", "equals": "{{token}}" }
  ],
  "pause": "20ms"
}
```

JSON selectors use JSON Pointer (RFC 6901): `/items/0/id` selects an array
element, `~1` escapes `/`, and `~0` escapes `~`. An empty pointer selects the
document root. A pointer must select a present, non-null string, number, or
boolean. Structured values, invalid JSON, missing keys, and noncanonical array
indexes fail verification. JSON numbers retain their full decimal precision.

An extraction uses exactly one of `json_pointer` or `header`. An assertion
uses one of:

- `json_pointer` and `equals`: exact scalar comparison.
- `header` and `equals`: comparison to the first header value.
- `body_contains`: literal substring matching.

Expected values can contain templates, including extractions from the same
response. Extractions are committed only if all extraction and assertion
checks pass. Missing headers fail even when the expected value is an empty
string. Each step supports at most 100 extractions and 100 assertions.

Body verification captures at most 1 MiB. Larger responses fail with
`capture_too_large`, while still streaming the body up to `max_response_bytes`
for connection reuse. Header-only verification does not capture the body.
Automatic decompression remains disabled; a compressed body cannot be checked
as JSON or text without changing the request's encoding behavior.

`pause` occurs after a successful step, including the final step, and belongs
to the journey's duration. `think_time` is a separate delay between journeys
in users mode. The per-request `timeout` includes the complete response body.
`journey_timeout` (default 30s) bounds the entire journey, including all pauses
and verification. At planned load completion, admitted journeys can finish
remaining steps during drain; interruption cancels them immediately.

## Sessions and authentication

Each virtual user owns a cookie jar throughout users-mode execution. Cookies
are shared only by that user's steps and iterations. Every rate-mode journey
gets a fresh jar. HTTP transports and connection pools are shared, while cookie
and variable state are isolated.

Cookie state is limited to 512 distinct keys and 64 KiB of stored name, value,
domain, and path bytes per session. Existing keys can update; explicit expiry
deletes a key. Natural expiry does not free the history key until an explicit
deletion arrives. A response exceeding either limit fails its step with
`cookie_limit` instead of silently continuing with an incomplete session.

All steps are confined to the configured origin, including dynamically
expanded URLs. This version does not support cross-origin login flows,
automatic redirects, OAuth helpers, external secret stores, or scripting.
Bearer and API-key headers can use initial variables or extracted values.
The demo's base64 tokens are test fixtures, not an authentication implementation.

## Mixes and arrivals

Scenario weights form a deterministic repeating cycle in configuration order.
For weights 3 and 1, arrivals select A, A, A, B, then repeat. Weights describe
scheduled demand; slower scenarios can have a different share of completed
journeys. Missed arrivals are attributed to their selected scenario efficiently,
including large skipped ranges.

Rate mode supports `constant` arrivals and seeded `jitter`. Jitter places one
arrival uniformly within each fixed rate interval and preserves
`ceil(duration × rate)` demand. The final partial interval is clipped to the
phase end. It is not a Poisson process. A seed reproduces intended times and
selection; operating-system timing and target responses remain variable.

The rate and concurrency cap apply to whole journeys, with sequential steps.
A full concurrency cap drops the arrival; it does not queue a journey. A late
scheduler discards older due arrivals instead of issuing a catch-up burst.

## Metrics and acceptance

Schema 2 preserves top-level HTTP request aggregates and adds `journeys`,
`scenarios`, and per-step results. With explicit scenarios:

- `journeys.scheduled = started + dropped_capacity + dropped_scheduler`.
- Top-level request `scheduled = started = completed` after draining. There
  are no request drops because unstarted journeys do not create HTTP attempts.
- A failed template is a logical request attempt but sends no network request;
  the step records `template`. Other new errors are `assertion`, `extraction`,
  `capture_too_large`, and `cookie_limit`.
- A skipped step is not a request attempt. Its `skipped` count refers only to
  admitted journeys stopped by an earlier failure, cancellation, or deadline.
- Journey latency includes the sequence, pauses, and verification. Per-step
  latency ends at body completion or transport/template failure, before
  extraction and assertions. Journey scheduled latency includes admission delay.
- For subsequent steps, request scheduled latency starts when that step becomes
  eligible. Only the first step inherits the journey arrival time.
- Every journey and its requests belong to the phase of journey admission.
  Per-scenario average throughput divides by the entire run's load window,
  not just time when that scenario happened to run.

The existing top-level `thresholds` remain request checks, except
`max_drop_percent`, which always checks workload arrival drops. Add
`journey_thresholds` for whole-journey checks. The same fields are supported
under each scenario's `thresholds`:

| Field | Unit |
| --- | --- |
| `p95` | Maximum full journey latency, e.g. `500ms` |
| `max_error_percent` | Failed journeys / completed journeys × 100 |
| `max_drop_percent` | Missed journeys / scheduled journeys × 100 |
| `min_started_per_second` | Minimum admitted journeys per second |

Global journey errors default to a maximum of 0%; explicit scenario thresholds
have no defaults. Use `null` to disable a percentage check. A configured
latency/error check requires completed samples; a drop check requires scheduled
demand. An unvisited scenario shows NO SAMPLES in HTML. Its configured checks
can fail for missing samples. Scenario and global checks affect the CLI exit
code. A journey timing out during a pause fails journey checks even if all its
HTTP requests succeeded. Legacy single-request tests keep their original
request metrics and thresholds.
