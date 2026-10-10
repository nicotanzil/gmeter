# App screenshot capture notes

[Documentation](../README.md) · [Project overview](../../README.md)

These are real captures of the embedded app from the development source checkout
on 9 October 2026, displaying `0.6.0-dev`. They use only the bundled local demo and
an isolated workspace. No private target, user data, or workload credentials
appear in the images. Numeric measurements are actual observations of that demo
run and must not be presented as performance benchmarks.

| Image | State | Capture |
| --- | --- | --- |
| [test-builder.jpg](test-builder.jpg) | Validated Local API baseline configuration and planned 800 arrivals | Full page, 1280 × 1917 |
| [run-results.jpg](run-results.jpg) | Completed baseline, pass criteria, and four workload charts | Document crop from the top, 1280 × 1360; later diagnostic sections omitted |
| [user-guide.jpg](user-guide.jpg) | User guide, first-test instructions, and practice summary | Viewport, 1280 × 720 |

The browser's normal viewport was 1280 × 720; it was not resized for these
captures. Screenshots are JPEG with no pixel edits or substituted metrics.
The source checkout included the pending guided builder, insecure HTTPS, and
user-guide work; the screenshots do not represent an older tagged release.

## Reproduce or refresh

1. Build the current source with `make build` (or the equivalent Go commands).
2. Start `./bin/gmeter-demo` with its default address, delay, and capacity.
3. Start `./bin/gmeter ui -listen 127.0.0.1:8792 -data work/docs-preview` with a
   dedicated workspace. If reusing it, retain existing test evidence as needed.
4. In **Build a test**, start from **Local smoke test**. For the pictured baseline,
   set name `Local API baseline`, rate `100`, duration `8s`, concurrency `16`,
   timeout `1s`, p95 `200ms`, maximum errors/drops `0`, and minimum started rate `99`.
   Keep diagnostics enabled, constant spacing, no workers/telemetry, and default
   certificate verification. Hide the optional welcome tip, then **Validate**.
5. Return to the top of **Build a test** and capture the configured builder.
6. Start the local run and wait for its actual final state. Inspect errors and
   missed demand before capturing **Live run**. Preserve the title, verdict,
   metrics, and charts in the result image; do not edit their values.
7. Open **User guide** from the sidebar and capture its overview and first steps.
8. Inspect each file for legibility, clipping, and sensitive content. Update
   captions, capture date/dimensions, and relevant doc text if the UI changed.

Keep assets in this directory so GitHub and offline checkouts resolve them.
Update all three together when navigation or visual structure changes. Use
fresh local data and describe any failed/incomplete state accurately.
