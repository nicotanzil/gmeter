# Bundled JMeter parity program

[Documentation](README.md) · [Project overview](../README.md)

The target is to cover the capabilities of **bundled Apache JMeter 5.6.3**, then
add useful capabilities beyond that baseline. This is an accepted product goal,
not the current state: gmeter remains an HTTP load-testing application. The
[official download](https://jmeter.apache.org/download_jmeter.cgi) and manual
were checked on 8 October 2026. No third-party plugin baseline is included yet.

The dashboard's **Feature coverage** view lists 68 capability families, their
current limitations, sources, and delivery stages. Its canonical inventory is
[coverage.json](../internal/dashboard/assets/coverage.json). Status meanings:

- **Available:** the specific capability stated in the row is implemented.
- **Partial:** a related capability exists, but the listed gaps remain.
- **Not available:** implementation is absent; the stage is a backlog assignment.

These are family-level labels, not a percentage of JMeter compatibility. A
working JSON Pointer extractor does not establish JSONPath parity. An HTTP
client sending XML does not establish XML assertion parity. Timing out requests
does not implement independent duration assertions. This audit does not establish
better throughput, accuracy, resource use, or target health.

## Current comparison

| Area | gmeter today | Work needed for JMeter coverage |
| --- | --- | --- |
| HTTP testing | Requests, sequential journeys, cookies, raw payloads, variable templates, header/scalar JSON extraction, basic assertions; verified or explicitly insecure HTTPS | Redirect chains, richer authentication, uploads, connection policies, additional selectors and assertions, client certificates |
| Workloads | Fixed users, constant/jittered arrivals, rate phases, weighted journeys, bounded admission and explicit drops | User ramps and groups, finite loops, conditional control flow, synchronization, setup/teardown, additional timer distributions |
| Data and customization | Inline rows and scoped strings | File datasets, function library, processor hooks, scripting and extension APIs |
| Other protocols | HTTP(S) | Database, directory, messaging, TCP, transfer, mail, process and Java adapters; Bolt; legacy support decisions |
| Authoring and compatibility | Local web app, JSON plans, saved runs and portable bundles | Visual journey editor, recording, component tree/scoping, JMX migration and external result exchange |
| Analysis | Bounded histograms, phase/scenario/step metrics, portable reports, comparisons, generator diagnostics and target metric observation | Debug samples, configurable result sinks, more charts, offline sample analysis, notification listeners |
| Distributed execution | A global rate partitioned across authenticated workers | Distributed users, file deployment, recovery, and compatibility with existing distributed plans |

The comparison follows Apache's [protocol overview](https://jmeter.apache.org/),
[web test guide](https://jmeter.apache.org/usermanual/build-web-test-plan.html),
[component reference](https://jmeter.apache.org/usermanual/component_reference.html),
[functions reference](https://jmeter.apache.org/usermanual/functions.html),
[report guide](https://jmeter.apache.org/usermanual/generating-dashboard.html), and
[remote testing guide](https://jmeter.apache.org/usermanual/remote-test.html).
Detailed rows link directly to the relevant official manual pages.

## Delivery order and acceptance gates

### 1. Finish HTTP authoring and workflow coverage

- [x] Guided Request / Traffic / Pass criteria builder with a plan preview.
- [x] Graphical headers, raw bodies, success statuses, phases and request thresholds.
- [x] Opt-in insecure workload HTTPS, explicit result labeling, worker propagation.
- [x] Searchable in-app comparison with honest availability and official references.
- [ ] Visual journey/step editor, assertions, extraction, data and variable controls.
- [ ] File-backed CSV datasets with deterministic local/fleet allocation and EOF rules.
- [ ] Regex, boundary, CSS, XPath, JSONPath and JMESPath extraction and assertions.
- [ ] Authentication helpers, credentials from environment, redirects, form/multipart uploads.
- [ ] Custom workload CAs, mutual TLS, proxy/DNS/cache/connection settings.
- [ ] Recording/import pipeline with filters, correlation review and capture redaction.

Gate: create an authenticated, parameterized workflow in the UI without editing
JSON; import/export preserves it; local and distributed reference fixtures verify
exact request contents, session isolation, failures and skips. Recorder work must
follow Apache's [recording model](https://jmeter.apache.org/usermanual/jmeter_proxy_step_by_step.html)
with explicit certificate and capture lifecycle; no implicit machine trust changes.

### 2. Add execution semantics and debugging

- [ ] Typed flow nodes: sequence, finite/conditional loop, branches, foreach,
  once-only, switch, interleave/random selection, transaction and runtime limits.
- [ ] Scope/inheritance rules, shared fragments, setup/teardown and failure actions.
- [ ] User ramps, group lifecycle, additional timers, barriers and critical sections.
- [ ] Distributed users and data deployment; worker failure/recovery contracts.
- [ ] Bounded opt-in debug request/response samples, assertion traces and redaction.

Gate: deterministic fixtures prove loop counts, scoping, branch selection,
cancellation, setup/teardown, transaction metrics and timer behavior. Keep queues,
iteration time, retained samples and script execution bounded. Existing demand
accounting must not silently change when control flow is introduced.

### 3. Add protocol adapters

- [ ] A sampler interface with shared lifecycle, cancellation, metrics and assertions.
- [ ] SQL/JDBC; JMS publish/subscribe/request-reply; directory operations.
- [ ] TCP framing; FTP (secure transfer as an additional capability); SMTP and mailbox operations; Bolt.
- [ ] OS processes, Java objects/JUnit and access-log replay.
- [ ] Utility equivalents for mirror/debug/property inspection and login/default managers.

Gate: each adapter has a real disposable integration fixture, protocol-specific
configuration, test authoring, reports and distributed execution. Validate
[database pooling](https://jmeter.apache.org/usermanual/build-db-test-plan.html)
and [JMS provider behavior](https://jmeter.apache.org/usermanual/build-jms-point-to-point-test-plan.html)
against actual providers, not mock-only tests.

Go remains the coordinator and native engine. Arbitrary JDBC drivers, JMS
providers, Java samplers, JUnit, Groovy/JSR223 and BeanShell need a JVM execution
bridge if existing Java behavior is part of compatibility. Native Go adapters
alone cannot fulfill those requirements. The bridge is a proposed architecture,
not implemented or benchmarked. Use a versioned process protocol, independent
resource accounting, bounded payloads and cancellation deadlines.

### 4. Compatibility, scripting and result integrations

- [ ] JMX component/property inventory tied to the target release; component-level
  support matrix and conformance fixtures for every bundled entry and important option.
- [ ] JMX migration with explicit errors for unsupported semantics; no silent drops.
- [ ] JTL/CSV/XML import and export, offline sample analysis and full report coverage.
- [ ] Script hooks and function library; public sampler/timer/processor/listener APIs.
- [ ] External metric sinks, mail/custom listeners and extension lifecycle controls.
- [ ] Decide compatibility for deprecated components, including legacy MongoDB.

Gate: run representative reference plans in both engines and compare observable
semantics. The grouped catalog is not a substitute for this component/property
inventory. Preserve the native fast path; make optional runtimes explicit.
Apache's [best practices](https://jmeter.apache.org/usermanual/best-practices.html)
and [backend listener documentation](https://jmeter.apache.org/usermanual/realtime-results.html)
are reference points for execution and integration tests.

### 5. Beyond the baseline and evidence of advantages

- [ ] WebSocket and gRPC with message/stream semantics and per-operation metrics.
- [ ] OpenTelemetry export and trace correlation.
- [ ] Repeatable baseline trials and statistical regression analysis.
- [ ] Reproducible JMeter comparison harness: matched payloads, connections, TLS,
  reporting, hardware, load shape and independently observed target counts.

Already useful gmeter capabilities include explicit scheduler/capacity misses,
fleet-wide rate partitioning, bounded aggregates, local portable run history,
and independent target metric coverage. These are implementation facts, not
claims that JMeter or its plugin ecosystem cannot provide related functionality.

## Completion definition

“JMeter is a subset of gmeter” may be claimed only after every in-scope bundled
component and relevant configuration behavior has a tested native equivalent or
a supported compatibility path. Equivalent workflow outcomes must include CLI,
UI authoring, persistence, reports and distributed execution where applicable.
Named differences, unsupported settings, deprecated components and Java behavior
cannot be hidden behind an “Available” family label. Record the exact JMeter
release and adapter versions. Expand scope explicitly for third-party plugins.
Until those gates pass, describe gmeter as pursuing parity.
