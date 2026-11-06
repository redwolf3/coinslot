# Development guidelines

## Workflow

- Work on a branch; `main` is updated through pull requests.
- **Small, shippable commits.** Each commit builds and passes:
  ```sh
  gofmt -l .                                     # prints nothing
  go vet ./... && GOOS=windows GOARCH=amd64 go vet ./...
  go test ./...
  GOOS=windows GOARCH=amd64 go build ./...
  ```
- Commit subject: `<area>: <what changed>`, where area is `core`, `collector`, `windows`,
  `cmd`, `docs`, `ci`, or `config`. The body explains *why* when it isn't obvious.
- CI (`.github/workflows/ci.yml`) runs on pull requests and on pushes to `main` (not on other
  branch pushes, so a PR doesn't run twice). It runs the same checks on Linux, plus a Windows job that runs
  the real binary (`procs`, `once`, a short `run`, `summary`) and uploads the deployable build.
  That Windows job is the only automated check of the Win32 code.
- Ask the owner before a large design change; record the outcome as an ADR.

## Where code goes

| Change | Location | Test with |
|--------|----------|-----------|
| Rule/counting/summary logic | `core/activity`, `core/summary` | synthetic logs via `core/internal/synth` |
| Config fields, matching | `core/config` | table tests on `Parse`/`Match` |
| Record fields | `core/record` (+ collector writes them) | collector tests + `windows/cmd/coinslot/summary_test.go` |
| Sampling loop, process diffing | `windows/collector` | fake `Probe` |
| A new OS fact | `windows/internal/win32` (read only) + `Probe` interface | CI smoke step; real PC |

Keep decisions out of `win32`. If code there has an `if` about apps, games, or thresholds,
it belongs in `core`.

## Testing

- Core tests build logs with `synth.New(...).Start()`, `Launch`, `Samples`, `Exit`, `Gap`.
  Prefer a realistic scripted scenario with exact expected minutes over mocks of internals.
- Every bug fixed in counting gets a synthetic-log test that reproduced it (e.g. the midnight split).
- Collector tests use a fake `Probe`. The end-to-end test runs the real loop through `FileSink`
  into `summary`; keep it passing. It guards the writer/reader contract.
- Tests must not depend on the host time zone. Use fixed zones, or `time.Local` only where the
  result doesn't depend on it.

## Changing the record schema

- Within `v: 1`, changes are **additive only**: new optional fields (`omitempty`), never
  renamed, removed, or re-typed fields. Old logs must still parse and summarise.
- A breaking change bumps `record.SchemaVersion` and needs an ADR plus a reader for the old
  version, or an explicit decision to drop it.
- Document every field in `docs/design.md` in the same commit.

## Changing config

- Additive fields with defaults applied in `config.Parse`. Validate in `Validate`.
- Process names in `config/config.example.json` stay marked `UNVERIFIED` until confirmed on
  the PC. Never hard-code process names in Go code.

## Win32 bindings

- Standard-library `syscall` only (ADR 0002). Check struct layouts and signatures against Microsoft
  docs; note the API name in a comment.
- `syscall.NewCallback` slots are finite and never freed. Create callbacks once, at package level.
- Read-only facts only. No window creation or hooks without an ADR.

## Documentation

- `design.md` describes the current system; update it with the code.
- New decisions go in `docs/adr/`, research in `docs/analysis/`, future work in `docs/roadmap.md`.
- Operator guides (`windows-setup.md`, `evening-test.md`) must match the CLI. Update them when
  flags or commands change.

## Privacy

Logs contain children's activity and window titles (browser page titles included). Never
commit real logs. Analysis notes summarise and redact.
