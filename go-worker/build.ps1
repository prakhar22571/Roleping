# Builds the Go worker to WASM for Cloudflare Workers.
# Scoped env var overrides here (not set globally) because TinyGo 0.38.0 only
# supports Go up to 1.24, which is older than whatever Go version is the
# system default - see RUNBOOK.md for why these specific pins matter.
$ErrorActionPreference = "Stop"

# Resolved by absolute path rather than relying on PATH, because wrangler
# spawns this script's `npm run build` in a child process that doesn't
# reliably inherit PATH updates made after the parent shell was opened
# (a real, repeatedly-hit issue on Windows - a "restart your terminal" fix
# doesn't stick once wrangler is the one doing the spawning).
$goRoot = "C:\Users\nikhi\sdk\go1.24.13"
$wasmOpt = "C:\Users\nikhi\tools\binaryen\bin\wasm-opt.exe"
$templExe = "$env:USERPROFILE\go\bin\templ.exe"
$tinygoExe = "C:\Users\nikhi\AppData\Local\Microsoft\WinGet\Packages\tinygo-org.tinygo_Microsoft.Winget.Source_8wekyb3d8bbwe\tinygo\bin\tinygo.exe"

foreach ($pair in @(
	@{ Path = $goRoot; Label = "Go 1.24.13" },
	@{ Path = $wasmOpt; Label = "wasm-opt" },
	@{ Path = $templExe; Label = "templ CLI" },
	@{ Path = $tinygoExe; Label = "tinygo" }
)) {
	if (-not (Test-Path $pair.Path)) {
		throw "$($pair.Label) not found at $($pair.Path) - see RUNBOOK.md for install steps."
	}
}

$env:PATH = "$goRoot\bin;$env:PATH"
$env:GOROOT = $goRoot
$env:GOTOOLCHAIN = "local"
$env:WASMOPT = $wasmOpt

Set-Location $PSScriptRoot

# -lazy: only rewrite a _templ.go file if its .templ source is newer.
# Without this, `templ generate` rewrites every output file on every build
# regardless of whether anything changed, which touches files under
# internal/web/ that wrangler's watcher (build.watch_dir includes "internal")
# treats as a source change - triggering another build, which regenerates
# the same files again, forever. This is what an apparently-endless
# "still building" session actually is.
& $templExe generate -lazy
if ($LASTEXITCODE -ne 0) { throw "templ generate failed" }

go run github.com/syumai/workers/cmd/workers-assets-gen
if ($LASTEXITCODE -ne 0) { throw "workers-assets-gen failed" }

& $tinygoExe build -o ./build/app.wasm -target wasm -no-debug -interp-timeout=8m ./...
if ($LASTEXITCODE -ne 0) { throw "tinygo build failed" }

Write-Host "Build complete: build/app.wasm"
