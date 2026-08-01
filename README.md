# Roleping

Free-hosted job alert system: register company job boards (Amazon, Greenhouse, Lever), poll them every 15 minutes, use two OpenRouter LLMs to flag SDE I / 1+ year experience postings, and get emailed via Resend. Server-rendered dashboard for managing companies and tracking application status, behind a password login.

Written in Go, compiled to WebAssembly via TinyGo, running entirely on Cloudflare Workers + D1 + KV (free tier).

## Multi-user

Several people can share one deployment. Sign-in is email + password, handled by the app itself: the owner adds people on the Users page, the app generates a password and shows it once, and they can change it from their Account page afterwards.

Job postings and their LLM verdicts are shared, so each posting is fetched and classified once no matter how many people are watching that company. Everything personal is per-user: which companies you subscribe to, your application status for each job, and your notifications. Email alerts go only to the owner (`OWNER_EMAIL`), a limitation of Resend's free sender; everyone else sees their alerts on the dashboard. See `go-worker/RUNBOOK.md` for the full model and its rationale.

## Structure

- `go-worker/` — the whole app: HTTP API + server-rendered dashboard (templ + htmx) + 15-minute cron pipeline, one Go binary
- `migrations/` — D1 schema migrations
- `go-worker/RUNBOOK.md` — toolchain setup (Go/TinyGo/templ version pins — read this first), Cloudflare setup, adding companies, rotating models, troubleshooting

## Development

See `go-worker/RUNBOOK.md` for one-time toolchain and Cloudflare setup (D1, KV, secrets, Access, and the specific Go/TinyGo/templ versions this needs). Once set up:

```
cd go-worker
npm install
npm run build      # templ generate -> workers-assets-gen -> tinygo build
npm run dev         # wrangler dev
npm run deploy       # wrangler deploy
```
