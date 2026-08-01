# Builds the Go worker to WASM for Cloudflare Workers.
# Scoped env var overrides here (not set globally) because TinyGo 0.38.0 only
# supports Go up to 1.24, which is older than whatever Go version is the
# system default - see RUNBOOK.md for why these specific pins matter.
$ErrorActionPreference = "Stop"

$goRoot = "C:\Users\nikhi\sdk\go1.24.13"
$wasmOpt = "C:\Users\nikhi\tools\binaryen\bin\wasm-opt.exe"

if (-not (Test-Path $goRoot)) {
	throw "Go 1.24.13 not found at $goRoot - see RUNBOOK.md for install steps."
}
if (-not (Test-Path $wasmOpt)) {
	throw "wasm-opt not found at $wasmOpt - see RUNBOOK.md for install steps."
}

$env:PATH = "$goRoot\bin;$env:PATH"
$env:GOROOT = $goRoot
$env:GOTOOLCHAIN = "local"
$env:WASMOPT = $wasmOpt

Set-Location $PSScriptRoot

templ generate
if ($LASTEXITCODE -ne 0) { throw "templ generate failed" }

go run github.com/syumai/workers/cmd/workers-assets-gen
if ($LASTEXITCODE -ne 0) { throw "workers-assets-gen failed" }

tinygo build -o ./build/app.wasm -target wasm -no-debug -interp-timeout=8m ./...
if ($LASTEXITCODE -ne 0) { throw "tinygo build failed" }

Write-Host "Build complete: build/app.wasm"
