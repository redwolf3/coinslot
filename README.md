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
| `core/`     | Platform-independent logic: config, record schema, (soon) classifier, counter, summaries |
| `windows/`  | Windows collector (`coinslot.exe` / `coinslotw.exe`), Win32 probe, packaging and install scripts |
| `config/`   | Example config mapping process names to metered/allowed, player/launcher |
| `docs/`     | Design notes and run instructions                            |
| `ios/`      | Placeholder (later)                                          |
| `service/`  | Placeholder (later)                                          |

## Quick start (dev)

```sh
go test ./...
windows/scripts/package.sh   # -> dist/coinslot/: both exes, config.json, install scripts
```

See [docs/design.md](docs/design.md) for the design and record schema, and
[docs/windows-setup.md](docs/windows-setup.md) to install and collect data.

## License

Apache-2.0. See [LICENSE](LICENSE).
