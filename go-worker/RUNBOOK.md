# Runbook (Go / TinyGo version)

Roleping is a single Go binary (compiled to WASM via TinyGo) running on Cloudflare Workers, serving both the server-rendered dashboard (via [templ](https://templ.guide) + [htmx](https://htmx.org)) and the 15-minute cron pipeline.

## Toolchain versions — read this first

Getting this to build required pinning specific, mutually-compatible versions after hitting real incompatibilities:

| Tool | Version | Why this exact version |
|---|---|---|
| TinyGo | **0.38.0** | TinyGo 0.41.1 (latest at time of writing) has a broken `net/http` WASM patch (`roundtrip_js.go` references a `Transport.roundTrip` method that no longer exists) — compiling anything that imports `net/http` fails. 0.38.0 doesn't have this regression. |
| Go | **1.24.13** | TinyGo 0.38.0 only supports Go 1.19–1.24. A newer system-default Go (e.g. 1.25+/1.26+) will not work with this TinyGo version. |
| `a-h/templ` (go.mod) | **v0.3.833** | Needs Go ≥1.23 (compatible with 1.24.13) — the latest templ (v0.3.1020) requires Go ≥1.25. |
| `templ` CLI | **v0.3.833** | Must match the runtime version in go.mod exactly — a newer CLI generates code calling functions that don't exist in an older runtime. |
| Binaryen `wasm-opt` | any recent release (131 used here) | TinyGo's `-target wasm` requires `wasm-opt` for the final build step; **not** the same as the `binaryen` npm package, which on Windows is just a Node.js wrapper script, not a native binary. |

These pins are **not** applied to your system's default Go/PATH — see below. If Cloudflare/TinyGo/templ release compatible newer versions in the future, this table should be revisited; don't assume the pins are permanent.

## One-time toolchain setup

1. **Go (system default)** and **TinyGo** — installed via winget already if you're continuing this session:
   ```
   winget install --id GoLang.Go -e
   winget install --id tinygo-org.tinygo --version 0.38.0 -e
   ```

2. **Go 1.24.13 side-by-side** (required because TinyGo 0.38.0 caps at Go 1.24, but your system Go is likely newer):
   ```
   go install golang.org/dl/go1.24.13@latest
   go1.24.13 download
   ```
   This installs to `%USERPROFILE%\sdk\go1.24.13`. Do **not** set this as your global `GOROOT` — it's scoped to this project's build only (see `build.ps1`), so it doesn't silently break other Go projects on this machine that expect the newer default Go.

3. **templ CLI, pinned to match go.mod**:
   ```
   go install github.com/a-h/templ/cmd/templ@v0.3.833
   ```

4. **Binaryen (`wasm-opt`)** — download the real native Windows binary from GitHub releases (not the npm `binaryen` package, which is a JS shim on Windows):
   ```
   Invoke-WebRequest -Uri "https://github.com/WebAssembly/binaryen/releases/download/version_131/binaryen-version_131-x86_64-windows.tar.gz" -OutFile "$env:TEMP\binaryen.tar.gz"
   mkdir C:\Users\<you>\tools\binaryen
   tar -xzf "$env:TEMP\binaryen.tar.gz" -C C:\Users\<you>\tools\binaryen --strip-components=1
   ```
   If your `wasm-opt.exe` ends up somewhere other than `C:\Users\nikhi\tools\binaryen\bin\wasm-opt.exe`, update the path in `build.ps1`.

5. **Cloudflare account setup**:
   - `wrangler login`
   - `wrangler d1 create roleping-db` — copy the returned `database_id` into `wrangler.jsonc`'s `d1_databases[0].database_id`, replacing `REPLACE_WITH_D1_DATABASE_ID`.
   - `wrangler d1 migrations apply roleping-db --local` then `--remote` (migrations live in `../migrations/0001_init.sql`, shared schema).
   - `wrangler kv namespace create CONFIG_KV` — copy the returned `id` into `wrangler.jsonc`'s `kv_namespaces[0].id`, replacing `REPLACE_WITH_KV_NAMESPACE_ID`.
   - Seed model config (check current free models at https://openrouter.ai/models first): `wrangler kv key put --binding=CONFIG_KV model_a_id "<model-id>"` and same for `model_b_id`.
   - `wrangler secret put OPENROUTER_API_KEY` and `wrangler secret put RESEND_API_KEY` (never commit these; mirror them in a gitignored `.dev.vars` for local dev).
   - Resend: sign up, verify your own email as recipient — `onboarding@resend.dev` (already set as `ALERT_FROM_EMAIL` in `wrangler.jsonc`) can send to your verified account email with no domain purchase needed.
   - **Cloudflare Access** (gates the dashboard behind email login): Zero Trust → Access → Applications → Add → Self-hosted. Domain = your deployed Worker's hostname. Path scope = the whole hostname (`/*`) so both the dashboard pages and `/api/*` are protected. Policy: Allow, Include = Emails → your address(es). Auth method: One-time PIN (email OTP), no separate identity provider needed.

## Build & run

```
npm install                 # installs wrangler (the only Node dependency)
npm run build                # runs build.ps1: templ generate -> workers-assets-gen -> tinygo build
npm run dev                  # wrangler dev (rebuilds via [build].command automatically)
npm run deploy                # wrangler deploy
npm run db:migrate:local
npm run db:migrate:remote
```

`build.ps1` is what actually does the work — it scopes `GOROOT`, `GOTOOLCHAIN=local`, and `WASMOPT` to just that script's process, then runs:
```
templ generate
go run github.com/syumai/workers/cmd/workers-assets-gen
tinygo build -o ./build/app.wasm -target wasm -no-debug ./...
```
`wrangler.jsonc`'s `"build": {"command": "npm run build"}` means `wrangler dev`/`wrangler deploy` invoke this automatically — you don't need to run it manually before deploying, just before manually inspecting `./build/app.wasm`.

Current build output: ~1.4-1.7MB WASM (~600KB gzipped) — comfortably under Cloudflare's 3MB free-tier compressed script size limit.

**Build time is uneven and that's normal, not a hang**: TinyGo links with ThinLTO, which the first build (or any build after a large code change) can take 8-12 minutes for on this machine — it's genuinely working (LLVM optimizing/linking), not stuck. Once `%LOCALAPPDATA%\tinygo\thinlto` has a warm cache for the current code, rebuilds are usually ~40 seconds. Don't kill a build just because it's quiet for a few minutes; check for a `tinygo.exe` process actively consuming CPU before assuming it's hung.

## Testing locally

```
cd go-worker
npx wrangler d1 migrations apply roleping-db --local   # first time only
npx wrangler dev --port 8787
```

You do **not** need real `OPENROUTER_API_KEY`/`RESEND_API_KEY` values to test the adapter → dedupe → classification → notification pipeline end-to-end — without them, OpenRouter/Resend calls fail with 401s, and the pipeline's fail-open behavior kicks in exactly as designed (`match=true`, reasoning tagged `classification_failed: ...`, notification recorded as `failed` with the real error detail). This is a good way to confirm the plumbing works before wiring up real keys.

Add a real company and trigger a manual run:
```
curl -X POST http://127.0.0.1:8787/api/companies -H "Content-Type: application/json" \
  -d '{"name":"Figma","portal_url":"https://boards.greenhouse.io/figma","adapter_type":"greenhouse","adapter_config":{"boardToken":"figma"}}'

curl -X POST http://127.0.0.1:8787/api/run-now
curl http://127.0.0.1:8787/api/jobs?limit=5
```
Or just open `http://127.0.0.1:8787/jobs` and `http://127.0.0.1:8787/companies` in a browser — the dashboard works the same locally as deployed, no Access gate in local dev.

`wrangler dev`'s own log output on Windows is UTF-16 encoded when redirected to a file — `grep`/`findstr` for plain ASCII patterns like "Ready on" won't match against a redirected log; use `strings` (or just try connecting) instead if you're scripting around it.

## Architecture notes

- **One binary, no separate frontend build**: the dashboard is server-rendered via `templ` components (`internal/web/*.templ`) using [htmx](https://htmx.org) (loaded from a CDN `<script>` tag) for interactivity — form submissions and filter changes hit plain HTTP endpoints that return HTML fragments, not JSON. There's no React/Vite build step.
- **Both a JSON API and the HTML dashboard exist side by side**: `/api/*` routes return JSON (companies, jobs, verdicts, notifications, application-status, run-now) for programmatic use; `/companies`, `/jobs`, `/jobs/{id}` serve the HTML dashboard. They share the same `internal/db` query layer.
- **D1 access is via `database/sql`**: `sql.Open("d1", "DB")` (blank-imported driver from `github.com/syumai/workers/cloudflare/d1`), not a bespoke query builder. This driver is marked alpha upstream — if you see D1-related errors that don't reproduce in plain SQL, that's the first thing to suspect.
- **Cron**: `github.com/syumai/workers/cloudflare/cron` — `cron.ScheduleTaskNonBlock` registers the scheduled handler alongside `workers.ServeNonBlock` for the HTTP handler, both running in the same binary (see `main.go`).
- **Config per-request, not cached**: `config.Load()` is called fresh at the start of every handler/cron tick, not once at cold start — Cloudflare bindings (D1, KV, env vars) are tied to the current request's runtime context and can't be safely cached across invocations.

## Adding a company

Same shape as before, either via the dashboard's Companies page or `POST /api/companies`:
```json
{
  "name": "Example Co",
  "portal_url": "https://boards.greenhouse.io/exampleco",
  "adapter_type": "greenhouse",
  "adapter_config": { "boardToken": "exampleco" }
}
```
Adapter config shapes are unchanged: Greenhouse needs `boardToken`, Lever needs `company`, Amazon optionally takes `resultLimit`/`maxResults`.

## Rotating OpenRouter models

Same as before — write to the `CONFIG_KV` namespace, no redeploy needed:
```
wrangler kv key put --binding=CONFIG_KV model_a_id "<new-model-id>"
wrangler kv key put --binding=CONFIG_KV model_b_id "<new-model-id>"
```

## Troubleshooting

- **`templ generate` fails or generates code that won't compile**: check `templ version` matches the version in `go.mod`'s `require github.com/a-h/templ` line exactly.
- **`tinygo build` fails with a `net/http`/`roundtrip_js.go` error**: you're likely on a newer TinyGo than 0.38.0 that has the regression described above — reinstall the pinned version.
- **`tinygo build` fails with "requires go version X through Y, got Z"**: your `GOROOT`/`PATH` aren't pointing at the pinned Go 1.24.13 — check `build.ps1` is actually being invoked (not a stale `npm run build` alias) and that the paths inside it are correct for your machine.
- **`wasm-opt` not found**: confirm `WASMOPT` in `build.ps1` points at a real Windows `.exe` from a Binaryen GitHub release, not the npm `binaryen` package's wrapper script.
- **No emails arriving**: check the `notifications` table (or a job's detail page — it shows notification status/error). Also check Resend's own dashboard delivery log using the `resend_id`.
- **A company stopped returning new jobs**: adapters return an error on non-2xx responses, which the pipeline logs per-company without blocking others. Check Worker logs (`wrangler tail`).
- **Amazon adapter breaking**: `amazon.jobs/en/search.json` is an internal, undocumented endpoint and could add bot detection/rate limiting without notice — there's no official fallback in this Workers-only design if that happens.
- **Outbound requests fail with "Netdev not set", or hang and never respond**: any code path using a plain `&http.Client{}` (or `http.DefaultClient`) routes through Go's standard `net` package, which needs a "netdev" network device abstraction TinyGo's WASM runtime doesn't provide. All outbound requests (adapters, OpenRouter, Resend) must go through `config.NewFetchClient()` (`github.com/syumai/workers/cloudflare/fetch`) and its own `Request`/`Do` types instead of `net/http`'s — this is already how the codebase is written, but if you add a new outbound call, don't reach for a bare `http.Client`.
- **`wrangler dev` seems to rebuild in an endless loop** ("The file src changed, restarting build..." repeatedly): the watcher was seeing the build's own output (`build/app.wasm` etc.) as a source change and retriggering itself. `wrangler.jsonc`'s `build.watch_dir` is scoped to `["main.go", "go.mod", "internal"]` specifically to prevent this — if you see the loop again, confirm that setting is still in place and that no new source lives outside those paths.
- **A Greenhouse/Lever job's `qualifications_text` still contains `&lt;`/`&gt;` noise**: some sources (confirmed on Greenhouse) return their `content` field with HTML tags entity-encoded rather than literal — `stripHTML` (`internal/adapters/greenhouse.go`) unescapes entities before stripping tags for exactly this reason; if a new source needs the same treatment, reuse `stripHTML` rather than writing a fresh regex.
- **A large numeric ID from an external API gets mangled or a JSON unmarshal fails with an "overflows"/"cannot unmarshal number" error**: TinyGo's `int` is 32-bit on the `wasm` target (unlike typical 64-bit Go), so a struct field declared as bare `int` for something like a Greenhouse job ID (which can exceed 2^31) will fail once real data has a large enough ID. Use `int64` explicitly for any field backed by an external numeric ID, timestamp, or anything else that isn't a small bounded count.

