# 0002. Go, standard library only

- Status: Accepted
- Date: 2026-11-06

## Context

A thin Win32 collector plus pure logic, deployed to a shared family PC, with fast iteration as
the main goal. Development happens partly on Linux with no Windows machine at hand.

## Decision

Go, single module at the repo root. Win32 calls use the standard library's `syscall` package
(`LazyDLL`), no third-party dependencies.

## Alternatives considered

- **Python + pywin32/ctypes:** fastest to edit, but needs a runtime on the PC, and Win32 binding
  mistakes only show up at runtime on Windows.
- **C#:** good Windows APIs, but needs the .NET toolchain to build, and cross-building from
  Linux is less convenient.
- **Go + `golang.org/x/sys/windows`:** nicer bindings, but adds a dependency; the module
  checksum service was also unreachable from the dev sandbox. The handful of calls needed are
  easy to bind directly.

## Consequences

- One static `.exe`, nothing to install. `GOOS=windows go build/vet` catches binding
  errors anywhere. Core tests run on any OS.
- Win32 struct layouts and call signatures are hand-written in `windows/internal/win32`;
  check them against Microsoft docs when adding calls.
- New dependencies need the owner's approval.
