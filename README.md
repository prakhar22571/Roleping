# Roleping

Free-hosted job alert system: register company job boards (Amazon, Greenhouse, Lever), poll them every 15 minutes, use two OpenRouter LLMs to flag SDE I / 1+ year experience postings, and get emailed via Resend. Server-rendered dashboard for managing companies and tracking application status, gated by Cloudflare Access.

Written in Go, compiled to WebAssembly via TinyGo, running entirely on Cloudflare Workers + D1 + KV (free tier).

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
