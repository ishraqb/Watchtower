# Deploying Watchtower for free

Watchtower runs two ways from the same codebase, picked by environment variables:

- **Local / dev** (unchanged): Kafka + Zookeeper + TimescaleDB via `docker compose`. This is the "real" architecture - just run `make up`.
- **Hosted (this guide)**: the same app, but with `BROKER=redis` and a plain Postgres, so it fits entirely on free tiers. No managed Kafka exists for free in 2026, so the broker swaps to Redis Streams; Neon doesn't ship the TimescaleDB extension, so `market_ticks` is a regular table. No user-facing features change.

Everything below is free and needs no credit card.

## What goes where

| Piece | Service | Notes |
| --- | --- | --- |
| Frontend (static SPA) | Cloudflare Pages or Netlify | free, permanent |
| Go backend | Render (free web service) | kept awake with a pinger |
| Sentiment worker | Render (free web service) | woken on demand |
| Postgres | Neon | free, permanent (Render's free PG expires after 30 days, so don't use it) |
| Redis (rate limit + broker) | Upstash | free, permanent |
| Keep-alive pinger | UptimeRobot | free |

## 1. Database - Neon

1. Create a project at [neon.tech](https://neon.tech) and copy the connection string (looks like `postgres://user:pass@host/db?sslmode=require`).
2. Apply the schema from your machine:

   ```bash
   make migrate DATABASE_URL="postgres://...your neon url..."
   ```

   The Timescale-specific bits skip themselves automatically on Neon.

## 2. Redis - Upstash

1. Create a Redis database at [upstash.com](https://upstash.com).
2. Copy the `rediss://` connection URL (TLS). This single instance handles both rate limiting and the Redis Streams broker.

## 3. Backend + worker - Render

The repo has a [render.yaml](render.yaml) blueprint that defines both services.

1. In the Render dashboard: **New > Blueprint**, point it at this repo. It picks up `render.yaml` and creates `watchtower-backend` and `watchtower-worker`.
2. Fill in the secret env vars (marked "fill in" in the blueprint) on **both** services as relevant:
   - `FINNHUB_API_KEY` - your Finnhub key
   - `DATABASE_URL` - the Neon URL
   - `REDIS_URL` - the Upstash URL
3. On the **backend** also set:
   - `ALLOWED_ORIGINS` - your frontend origin once you have it (step 4), e.g. `https://watchtower.pages.dev`
   - `WORKER_WAKE_URL` - the worker's URL + `/wake`, e.g. `https://watchtower-worker.onrender.com/wake`
4. Note the backend's public URL (e.g. `https://watchtower-backend.onrender.com`) - the frontend needs it.

`BROKER=redis` and `GIN_MODE=release` are already set in the blueprint.

## 4. Frontend - Cloudflare Pages or Netlify

The frontend builds to static files (`adapter-static`). Connect the repo and use:

- **Build command**: `npm run build`
- **Build directory / root**: `frontend`
- **Output directory**: `build`
- **Environment variables** (set at build time):
  - `VITE_API_BASE` = your backend URL, e.g. `https://watchtower-backend.onrender.com`
  - `VITE_WS_URL` = same host but `wss://` + `/ws`, e.g. `wss://watchtower-backend.onrender.com/ws`

After it deploys, copy the site URL back into the backend's `ALLOWED_ORIGINS` and redeploy the backend.

## 5. Keep the backend awake - UptimeRobot

Render free services sleep after 15 minutes idle, which would drop the live websocket. Create a free [UptimeRobot](https://uptimerobot.com) HTTP monitor hitting `https://watchtower-backend.onrender.com/healthz` every 5-10 minutes. (The worker is intentionally left to sleep - the backend wakes it via `WORKER_WAKE_URL` when an anomaly fires.)

## Things to expect on the free tier

- **Cold starts**: if the backend ever does sleep, the first visit takes ~1 minute to spin up.
- **Sentiment latency**: the worker is woken on demand, so the first sentiment card after a quiet period can take ~1 minute to appear. During active use it's quick.
- **Live prices only stream during US market hours** - same as local. Outside hours the dashboard shows the last close (via `/api/quote`).

## Environment variable reference

Backend:

- `BROKER` - `kafka` (local, default) or `redis` (hosted)
- `DATABASE_URL`, `REDIS_URL`, `FINNHUB_API_KEY`
- `ALLOWED_ORIGINS` - comma-separated frontend origins (no localhost in prod)
- `WORKER_WAKE_URL` - optional; worker `/wake` URL for the free-tier wake nudge
- `GIN_MODE=release`, `PORT` (Render sets this automatically)

Worker:

- `BROKER`, `DATABASE_URL`, `REDIS_URL`, `FINNHUB_API_KEY`, `PORT` (auto)

Frontend (build-time):

- `VITE_API_BASE`, `VITE_WS_URL`
