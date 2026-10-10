# Contributing to gmeter

[Project overview](README.md) · [Documentation](docs/README.md)

Keep changes focused, measurements explicit, and each development milestone
usable. The [roadmap](ROADMAP.md) and [JMeter parity program](docs/jmeter-parity.md)
record product scope and acceptance gates.

## Development setup

Use Go 1.24 or newer. There are no third-party Go dependencies or frontend package
installation steps. The dashboard is embedded from `internal/dashboard/assets/`.
Rebuild and restart the local dashboard after frontend changes, then reload the
browser. A browser refresh alone does not replace assets in an existing binary.

```sh
make check build
```

Without Make:

```sh
go vet ./...
go test -race ./...
go build -o bin/gmeter ./cmd/gmeter
go build -o bin/gmeter-demo ./cmd/gmeter-demo
```

Race-enabled tests require a Go-supported race-detector platform and its host C
toolchain. Tests start local fixture servers; loopback network access is needed.
The production binary has no Java or Node.js runtime requirement.

## Keep development state separate

Use a dedicated workspace when testing UI behavior:

```sh
./bin/gmeter-demo
# In another terminal:
./bin/gmeter ui -listen 127.0.0.1:8792 -data work/development
```

Do not run multiple dashboard processes against the same directory. `bin/`,
`results/`, `work/`, and the default `.gmeter/` directory are ignored. A custom
workspace elsewhere is not automatically ignored; avoid staging it or secrets.

## Before requesting review

- Explain the concrete behavior change and its limits. Connect it to a roadmap
  item when applicable.
- Run `gofmt` on changed Go files. Run `go vet ./...`, race-enabled tests, and
  builds for changes to executable behavior.
- Add regression coverage for measurement, cancellation, persistence, parsing,
  TLS, or concurrency behavior that changed. Use a local deterministic fixture;
  do not make tests depend on an external service.
- Check affected example files with `gmeter validate -config ...`.
- For dashboard changes, verify the complete affected journey in a real browser,
  check narrow layouts, and inspect warning/error logs. Preserve editor drafts
  and use isolated test history.
- Update user-facing docs and screenshots when behavior or labels change.
  Documentation-only changes need link, example, flag/default, and visual checks;
  avoid tests that only mirror prose or markup.
- Run `git diff --check`, inspect the diff, and keep generated binaries, scratch
  reports, credentials, and unrelated edits out of the change.

## Measurement invariants

Preserve the distinction between scheduled demand, admitted journeys, started
HTTP attempts, completed attempts, failed work, skipped steps, and missed demand.
Use monotonic elapsed time for scheduling and observation bounds. Do not invent
samples for work that did not run, average worker percentiles, or replace missing
telemetry with zero. See [measurement definitions](docs/measurements.md).

A performance claim needs repeatable evidence with comparable workloads, target
behavior, connections, TLS, hardware, and instrumentation. Local demo results
verify behavior; they are not performance benchmarks or proof of JMeter parity.

## Documentation and screenshots

Use relative Markdown links within the repository. Put user guides and reference
material under `docs/`, and versioned screenshots under `docs/images/`. Capture
real local-demo states with no private hosts, secrets, or personal data. Follow
[the capture notes](docs/images/README.md) when refreshing them.

The docs should describe implemented behavior. Mark partial/planned capabilities
explicitly and avoid unsupported production, compatibility, or security claims.

## Security and licensing

Use [SECURITY.md](SECURITY.md) for concerns that should not be disclosed publicly.
No license file is present in the current checkout. A maintainer license decision
is required before redistribution or a public contribution policy is finalized;
this guide does not assign a license or introduce a contributor agreement.
