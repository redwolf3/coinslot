# coinslot

Open-source, per-user **active-use** screen time meter.

Built-in parental tools count how long an app is *open*. Coinslot counts how long a
child is *actively playing*: the game's player process is in the foreground and
there has been recent input. Launchers, downloads, updates, and a game left open
in the background don't count.

## Status

v1 is **measurement only** on Windows: a per-user collector writes JSONL logs, and a
pure core turns them into a daily summary per child. No blocking, budgets, or tokens yet.

| Path        | What                                                         |
|-------------|--------------------------------------------------------------|
| `core/`     | Platform-independent logic: config, record schema, classifier, active-play counter, daily summary |
| `windows/`  | Windows collector (`coinslot.exe` / `coinslotw.exe`), Win32 probe, packaging and install scripts |
| `config/`   | Example config mapping process names to metered/allowed, player/launcher |
| `docs/`     | Design, roadmap, ADRs, analysis, guidelines, operator guides (index: `docs/README.md`) |
| `ios/`      | Placeholder (later)                                          |
| `service/`  | Placeholder (later)                                          |

## Quick start (dev)

```sh
go test ./...
windows/scripts/package.sh   # -> dist/coinslot/: both exes, config.json, install scripts
```

See [docs/design.md](docs/design.md) for the design and record schema,
[docs/windows-setup.md](docs/windows-setup.md) to install, and
[docs/evening-test.md](docs/evening-test.md) to collect an evening of data and check it.
Planned post-MVP work is in [docs/roadmap.md](docs/roadmap.md).
Decisions are recorded in [docs/adr/](docs/adr/), research in [docs/analysis/](docs/analysis/),
and development guidelines in [docs/guidelines/](docs/guidelines/development.md). AI agents start at
[AGENTS.md](AGENTS.md).

## License

Apache-2.0. See [LICENSE](LICENSE).
