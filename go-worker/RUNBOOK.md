# Runbook (Go / TinyGo version)

Roleping is a single Go binary (compiled to WASM via TinyGo) running on Cloudflare Workers, serving both the server-rendered dashboard (via [templ](https://templ.guide) + [htmx](https://htmx.org)) and the 15-minute cron pipeline.

## Deployment

| | |
|---|---|
| URL | https://roleping.prakhar-tools.workers.dev |
| Cloudflare account | prakhar.tools@gmail.com (`b527a50cabea48df5d40eaf764d6854a`) |
| D1 database | `roleping-db` (`183dbc14-f775-47a4-8637-3cf85357a283`, region APAC) |
| KV namespace | `CONFIG_KV` (`4350ba75f5fb4aa88711a3eaa961d4a0`) |
| Cron | `*/15 * * * *` |
| Bundle | ~1.57 MB raw / ~543 KB gzipped (limit 3 MB compressed) |

The D1 and KV IDs in `wrangler.jsonc` are account-scoped resource identifiers, not credentials — they're useless without an authenticated session, which is why they're committed rather than kept in secrets. The actual secrets (`OPENROUTER_API_KEY`, `RESEND_API_KEY`) are set via `wrangler secret put` and never live in the repo.

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

5. **Cloudflare account setup** (all free tier — 100k requests/day, 5GB D1, cron included):
   - `wrangler login`
   - `wrangler d1 create roleping-db` — copy the returned `database_id` into `wrangler.jsonc`'s `d1_databases[0].database_id`, replacing `REPLACE_WITH_D1_DATABASE_ID`.
   - `wrangler d1 migrations apply roleping-db --local` then `--remote` (migrations live in `../migrations/`, shared schema).
   - `wrangler kv namespace create CONFIG_KV` — copy the returned `id` into `wrangler.jsonc`'s `kv_namespaces[0].id`, replacing `REPLACE_WITH_KV_NAMESPACE_ID`.
   - Seed model config (check current free models at https://openrouter.ai/models first): `wrangler kv key put --binding=CONFIG_KV model_a_id "<model-id>" --remote` and same for `model_b_id`. Without `--remote` you only write the local dev KV.
   - `wrangler secret put OPENROUTER_API_KEY` and `wrangler secret put RESEND_API_KEY` (never commit these; mirror them in a gitignored `.dev.vars` for local dev).
   - `wrangler secret put SESSION_SECRET` — 32 random bytes, base64. This signs session cookies; rotating it signs everyone out.
   - Resend: sign up, verify your own email as recipient — `onboarding@resend.dev` (already set as `ALERT_FROM_EMAIL` in `wrangler.jsonc`) can send to your verified account email with no domain purchase needed. That address must match `OWNER_EMAIL`.
   - Seed the owner's password — see "Bootstrapping the very first owner" in the Authentication section below.

## Authentication and multi-user model

Login is **password-based and handled inside the app**. `internal/auth` resolves an identity for every request from a signed session cookie, and `auth.Middleware` (wrapped around the router in `main.go`) redirects anonymous browser requests to `/login` and returns 401 for anonymous `/api/*` calls.

Why not Cloudflare Access: Access is the better design — it keeps credential handling out of the app entirely — but activating even the free Zero Trust plan requires putting a **credit card on file**. Password auth avoids that. The code is written so Access can be re-adopted later without a rewrite (see `TRUST_ACCESS_HEADER` below).

How the pieces fit:

- **Password hashing** (`internal/auth/password.go`): PBKDF2-HMAC-SHA256, implemented against stdlib `crypto/hmac` rather than pulling in `golang.org/x/crypto`, to keep the WASM bundle small. Each password gets a 16-byte random salt; the per-user `password_iterations` column records the work factor so it can be raised later without invalidating existing hashes.
- **The work factor is deliberately low** (`DefaultIterations = 20000`). Cloudflare's free plan caps a Worker invocation at ~10ms CPU, and PBKDF2 in WASM eats that fast — set it too high and login fails outright rather than merely feeling slow. What carries the security is entropy, not iterations: generated passwords are 18 characters from a 56-character alphabet (~103 bits), which is not brute-forceable at any iteration count. **If you ever let users pick their own passwords, this trade-off stops holding** — revisit the iteration count and add a strength requirement.
- **Sessions** (`internal/auth/session.go`): the cookie is `base64(userID:epoch:expiry).HMAC-SHA256`, keyed by the `SESSION_SECRET` secret, `HttpOnly` + `Secure` + `SameSite=Lax`, 30-day expiry. Tampering with any field breaks the HMAC. `SESSION_SECRET` being unset is treated as fatal at login rather than falling back to a default key, which would make every cookie forgeable.
- **`session_epoch`** on `users` is bumped on every password change or owner reset, which immediately invalidates all previously issued cookies for that user. The handler doing the change re-issues its own cookie, so changing your password doesn't sign *you* out — just everyone else holding an older session.

Roles and account management:

- The **owner** is whoever's email matches the `OWNER_EMAIL` var — a config value, not a database flag. Only the owner sees the Users nav link and can reach `/users`.
- The owner adds people at `/users` by email. A password is generated and **shown exactly once** on that response; it is not recoverable afterwards, only re-generatable. Members change their own password at `/account`.
- The owner account cannot be deleted (it would leave nobody able to administer users), and a member cannot reach any `/users` route.
- **Bootstrapping the very first owner** is the one thing the UI can't do, since logging in requires a password that doesn't exist yet. Generate a hash offline with the same algorithm and insert it directly:
  ```
  wrangler d1 execute roleping-db --remote --command "INSERT INTO users (email, password_hash, password_salt, password_iterations) VALUES ('you@example.com','<hash>','<salt>',20000) ON CONFLICT(email) DO UPDATE SET password_hash=excluded.password_hash, password_salt=excluded.password_salt, password_iterations=excluded.password_iterations, session_epoch=session_epoch+1"
  ```
  Note `--command` executes only the **first** statement it's given; run multiple statements as separate invocations or use `--file`.

Two things not to get wrong:

- **Never set `TRUST_ACCESS_HEADER=true` unless a Cloudflare Access application is actually in front of the Worker.** That flag makes the app trust `Cf-Access-Authenticated-User-Email`, which is exactly right when Access sets it and a total authentication bypass when it doesn't — without the gate, anyone can send that header and become any user, including the owner. It defaults to off, and the app ignores the header entirely in that state.
- **`DEV_USER_EMAIL` must never appear in `wrangler.jsonc`'s `vars`.** It lives only in the gitignored `.dev.vars`, where it lets `wrangler dev` skip the login form. In production its absence is part of what makes anonymous requests anonymous. (It's currently left unset even locally, so local dev exercises the real login flow.)

What is shared vs. per-user:

| Data | Scope | Why |
|---|---|---|
| `companies` | Global | Anyone can add a company; everyone can see it. |
| `jobs`, `llm_verdicts` | Global | A posting is a fact about the world, and classification is user-agnostic — fetching and LLM-classifying once for all users is what keeps OpenRouter quota use flat as users are added. |
| `company_subscriptions` | Per user | Drives who gets notified. |
| `application_status` | Per user | Your "Applied" is not someone else's. |
| `notifications` | Per user | One row per (job, subscriber). |

**Subscription, not existence, drives alerts** — a company nobody is subscribed to is still fetched and classified, but produces no notifications. If you stop receiving emails, check you're actually subscribed on the Companies page.

**Email goes only to the owner.** `OWNER_EMAIL` (a var in `wrangler.jsonc`) identifies you; the pipeline sends a Resend email only to the subscriber whose address matches it, and records every other subscriber's notification with `email_status='none'` ("dashboard only" in the UI). This is a deliberate limitation of Resend's free `onboarding@resend.dev` sender, which can only deliver to the Resend account holder's own verified address. To email everyone, verify a domain in Resend, set `ALERT_FROM_EMAIL` to an address at that domain, and drop the owner check in `internal/jobs/pipeline.go`.

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
templ generate -lazy
go run github.com/syumai/workers/cmd/workers-assets-gen
tinygo build -o ./build/app.wasm -target wasm -no-debug ./...
```
`templ`, `tinygo`, `go`, and `wasm-opt` are all invoked by absolute path inside `build.ps1`, not via `PATH` — `wrangler dev` runs this in a child process that doesn't reliably inherit PATH changes made after your shell was opened, so relying on `PATH` here silently breaks depending on when you last opened a terminal.
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

curl -X POST http://127.0.0.1:8787/api/companies/1/subscription   # subscribe, or alerts never fire
curl -X POST http://127.0.0.1:8787/api/run-now
curl http://127.0.0.1:8787/api/jobs?limit=5
```
Or just open `http://127.0.0.1:8787/jobs` and `http://127.0.0.1:8787/companies` in a browser — the dashboard works the same locally as deployed, no Access gate in local dev.

**Identity in local dev**: local dev runs the real login flow. Sign in through `/login` in a browser, or with curl:
```
curl -X POST http://127.0.0.1:8787/login \
  --data-urlencode "email=you@example.com" --data-urlencode "password=..." -c cookies.txt
curl -b cookies.txt http://127.0.0.1:8787/api/me
# {"email":"you@example.com","is_owner":true,"user_id":1}
```
`GET /api/me` is the quickest confirmation of who the app thinks you are. Setting `DEV_USER_EMAIL` in `.dev.vars` skips the login form entirely if you'd rather not deal with cookies while iterating on something unrelated — but leave it unset when testing anything auth-shaped. (Editing `.dev.vars` doesn't reliably hot-reload; restart `wrangler dev`.)

Useful multi-user checks: the same job should show a different `application_status` per account, `/users` should 403 for a non-owner, and after a pipeline run the owner's notification should be `sent` while another subscriber's is `none`.

**The local D1 database is keyed by `database_id`** — editing that value in `wrangler.jsonc` (as happens when you first provision a real database) points `wrangler dev` at a fresh, empty local database, and your old local rows appear to vanish. They're still on disk under `.wrangler/state/v3/d1`, just under the previous id.

`wrangler dev`'s own log output on Windows is UTF-16 encoded when redirected to a file — `grep`/`findstr` for plain ASCII patterns like "Ready on" won't match against a redirected log; use `strings` (or just try connecting) instead if you're scripting around it.

## Architecture notes

- **One binary, no separate frontend build**: the dashboard is server-rendered via `templ` components (`internal/web/*.templ`) using [htmx](https://htmx.org) (loaded from a CDN `<script>` tag) for interactivity — form submissions and filter changes hit plain HTTP endpoints that return HTML fragments, not JSON. There's no React/Vite build step.
- **Both a JSON API and the HTML dashboard exist side by side**: `/api/*` routes return JSON (companies, jobs, verdicts, notifications, application-status, run-now) for programmatic use; `/companies`, `/jobs`, `/jobs/{id}` serve the HTML dashboard. They share the same `internal/db` query layer.
- **D1 access is via `database/sql`**: `sql.Open("d1", "DB")` (blank-imported driver from `github.com/syumai/workers/cloudflare/d1`), not a bespoke query builder. This driver is marked alpha upstream — if you see D1-related errors that don't reproduce in plain SQL, that's the first thing to suspect.
- **Cron**: `github.com/syumai/workers/cloudflare/cron` — `cron.ScheduleTaskNonBlock` registers the scheduled handler alongside `workers.ServeNonBlock` for the HTTP handler, both running in the same binary (see `main.go`).
- **Config per-request, not cached**: `config.Load()` is called fresh at the start of every handler/cron tick, not once at cold start — Cloudflare bindings (D1, KV, env vars) are tied to the current request's runtime context and can't be safely cached across invocations.
- **Auth is one `http.Handler` wrapper, not router middleware**: `main.go` does `auth.Middleware(router.New())`. The hand-rolled `internal/httprouter` deliberately has no middleware chain — there's exactly one cross-cutting concern and every route needs it, so wrapping the whole handler is simpler than adding a middleware slice. Handlers read the identity with `auth.FromContext(r.Context())`. The cron path doesn't go through HTTP at all, so it has no identity and uses `env.OwnerEmail` directly.

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
- **No emails arriving**: first check you're **subscribed** to the company (Companies page) — an unsubscribed company produces no notification at all, and this is the most common cause. Then check the `notifications` table (or a job's detail page — it shows notification status/error), and confirm the recipient matches `OWNER_EMAIL`, since only the owner gets email. Also check Resend's own dashboard delivery log using the `resend_id`.
- **Everything redirects to /login, or API calls return 401**: no session was resolved. Either you're genuinely signed out, your cookie predates a password change (`session_epoch` bump), or `SESSION_SECRET` was rotated/unset — rotating it invalidates every existing cookie, which looks exactly like a mass logout.
- **Login always fails with the right password**: check `SESSION_SECRET` is set on the Worker (`wrangler secret list`) — the login handler refuses to issue a cookie without it and returns a 500 rather than a 401, so a 500 here points at the secret, not the password. If the password was seeded by hand, confirm the offline hash generator used the same iteration count as `auth.DefaultIterations`.
- **A user exists but can't sign in**: their `password_hash` is NULL — created by the pipeline or an owner before a password was assigned. Use "Generate new password" on `/users`.
- **A user sees an empty dashboard or the wrong application statuses**: statuses, notifications and subscriptions are per-user by design; jobs and companies are global. If someone sees no jobs at all, they likely have the "My subscriptions only" filter on with no subscriptions. If two people see each other's application statuses, that's a real bug — check the per-user conditions in `ListJobs`/`GetJobListRow` are still in the `LEFT JOIN ... ON` clause and not moved into `WHERE` (which would silently turn them into inner joins and change the result set).
- **A company stopped returning new jobs**: adapters return an error on non-2xx responses, which the pipeline logs per-company without blocking others. Check Worker logs (`wrangler tail`).
- **Amazon adapter breaking**: `amazon.jobs/en/search.json` is an internal, undocumented endpoint and could add bot detection/rate limiting without notice — there's no official fallback in this Workers-only design if that happens.
- **Outbound requests fail with "Netdev not set", or hang and never respond**: any code path using a plain `&http.Client{}` (or `http.DefaultClient`) routes through Go's standard `net` package, which needs a "netdev" network device abstraction TinyGo's WASM runtime doesn't provide. All outbound requests (adapters, OpenRouter, Resend) must go through `config.NewFetchClient()` (`github.com/syumai/workers/cloudflare/fetch`) and its own `Request`/`Do` types instead of `net/http`'s — this is already how the codebase is written, but if you add a new outbound call, don't reach for a bare `http.Client`.
- **`wrangler dev` seems to rebuild forever and never actually settles** ("The file src changed, restarting build..." repeatedly, or a dev session that "looks stuck" for an hour+ but is actually looping): `wrangler.jsonc`'s `build.watch_dir` is scoped to `["main.go", "go.mod", "internal"]` to avoid watching `build/` output, but that still includes `internal/web/*_templ.go` — and by default `templ generate` rewrites every generated file on every build regardless of whether its content changed, which touches those files' mtimes and makes the watcher think source changed, triggering another build, which regenerates them again, forever. `build.ps1` runs `templ generate -lazy` specifically to prevent this (`-lazy` only regenerates a `_templ.go` file if its `.templ` source is actually newer) — if the loop reappears, confirm `-lazy` is still there and that a genuinely unrelated change isn't touching those files. If you ever see a `wrangler dev` session that accepts connections but never responds to *any* request (even `GET /`, which touches no DB or network), that's this loop mid-cycle, not a slow build or a code bug — kill it (`Stop-Process` on `wrangler`/`node`/`workerd.exe`, or just close the terminal) and start fresh.
- **A Greenhouse/Lever job's `qualifications_text` still contains `&lt;`/`&gt;` noise**: some sources (confirmed on Greenhouse) return their `content` field with HTML tags entity-encoded rather than literal — `stripHTML` (`internal/adapters/greenhouse.go`) unescapes entities before stripping tags for exactly this reason; if a new source needs the same treatment, reuse `stripHTML` rather than writing a fresh regex.
- **A large numeric ID from an external API gets mangled or a JSON unmarshal fails with an "overflows"/"cannot unmarshal number" error**: TinyGo's `int` is 32-bit on the `wasm` target (unlike typical 64-bit Go), so a struct field declared as bare `int` for something like a Greenhouse job ID (which can exceed 2^31) will fail once real data has a large enough ID. Use `int64` explicitly for any field backed by an external numeric ID, timestamp, or anything else that isn't a small bounded count.

