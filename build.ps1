$ErrorActionPreference = "Stop"

New-Item -ItemType Directory -Force -Path .\.go-cache, .\.go-tmp, .\.go-telemetry | Out-Null

$env:GOTELEMETRY = "off"
$env:GOTELEMETRYDIR = (Resolve-Path .\.go-telemetry).Path
$env:GOCACHE = (Resolve-Path .\.go-cache).Path
$env:GOTMPDIR = (Resolve-Path .\.go-tmp).Path

go build -ldflags="-H=windowsgui" -o Video2Text.exe .
