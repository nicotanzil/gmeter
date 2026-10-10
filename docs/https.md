# HTTPS workload options

[Documentation](README.md) · [Project overview](../README.md)

Workload HTTPS requests verify the certificate chain and hostname by default.
To test a development server with a self-signed, expired, or mismatched
certificate, explicitly opt in:

```sh
./bin/gmeter run -url https://localhost:8443/work -insecure -rate 10 -duration 2s
```

In the builder, expand **Request settings** and select **Skip HTTPS certificate
verification (insecure)**. In a scenario, set the top-level `"insecure": true`.
It applies to all workload requests, including journey steps and distributed
worker requests. Encryption is still used; server identity is not verified.
Use `-insecure=false` to override a scenario and restore verification.

The setting is saved in scenario configurations, result JSON, and run bundles.
CLI output, HTML reports, and the dashboard label runs with verification disabled.
Comparisons flag different verification settings. Bundle imports reject a
configuration/result mismatch. Older bundles without the field mean `false`.
The optional field is additive to existing result schemas 3–5.

Worker control and target telemetry use separate TLS clients; `insecure` does
not weaken either connection. Their existing `-worker-ca` and `-telemetry-ca`
options remain available. Custom CAs, client certificates, and selectable TLS
versions for **workload requests** remain in the parity backlog.
