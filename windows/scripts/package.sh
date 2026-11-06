#!/usr/bin/env bash
# Build a deployable folder: dist/coinslot/{coinslot.exe,coinslotw.exe,config.json,install.ps1,uninstall.ps1}
set -euo pipefail
cd "$(dirname "$0")/../.."
out=dist/coinslot
version=${VERSION:-$(git describe --always --dirty 2>/dev/null || echo dev)}
rm -rf "$out" && mkdir -p "$out"
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
go build -trimpath -ldflags "-X main.version=$version" -o "$out/coinslot.exe" ./windows/cmd/coinslot
go build -trimpath -ldflags "-X main.version=$version -H=windowsgui" -o "$out/coinslotw.exe" ./windows/cmd/coinslot
cp config/config.example.json "$out/config.json"
cp windows/scripts/install.ps1 windows/scripts/uninstall.ps1 "$out/"
echo "packaged $version -> $out"
