# Watchtower — Full Planning Document

> Last updated: June 18, 2026. This file captures all planning decisions made before implementation began. Continue from here on any device.

---

## What Is This Project

Watchtower is a live, event-driven financial intelligence platform. It monitors real-time stock market data, automatically detects unusual trading activity, and immediately explains it using news sentiment analysis. It also tracks US congressional stock disclosures and scores upcoming IPOs.

In plain English, the app:
- Shows live stock prices updating on a chart in real time
- Watches the market in the background and flags when something unusual happens (e.g. a huge spike in trading volume)
- The moment a spike is detected, it automatically fetches and reads recent news for that stock, scores the sentiment, and shows you the result on screen — within seconds, no page refresh
- Shows what US Congress members have been buying and selling, overlaid on stock charts
- Scores upcoming IPOs by risk level so you can see which ones look sketchy at a glance

---

## Resume Entry

**Project Title:** Watchtower — Real-Time Market Intelligence Platform

**Tech Stack:** Go, TypeScript, SvelteKit, Apache Kafka, TimescaleDB, PostgreSQL, Redis, Docker, Node.js, WebSocket

**Bullet Points:**
- Built an event-driven pipeline in Go and TypeScript where Apache Kafka decouples a real-time volume anomaly detector from an AFINN sentiment engine, correlating market spikes with live news within seconds
- Designed a polyglot microservices architecture backed by TimescaleDB for high-frequency tick ingestion, Redis distributed rate limiting, and a Go WebSocket server streaming live prices to a SvelteKit frontend

> Note: Do not add these to your resume until the features are actually built. Recruiters will ask about every bullet in detail.

---

## Key Decisions Made

### Language Stack
- **Go** — core backend, WebSocket server, anomaly detection, Kafka producer/consumer, all cron jobs
- **TypeScript (Node.js)** — sentiment microservice (replaces Python to show language versatility; other projects already use Python)
- **TypeScript (SvelteKit)** — frontend
- **No Python** in this project deliberately

### Sentiment Library
- Using the `sentiment` npm package (AFINN-based) in the Node.js worker
- Scores text on the same `-1.0` to `+1.0` compound scale as VADER
- Architecture is identical — just Node.js instead of Python

### Congressional Trading Data
- **Not** using Finnhub's `/stock/congressional-trading` endpoint (uncertain free-tier availability)
- Using two free, keyless public APIs instead:
  - House: `housestockwatcher.com/api/transactions`
  - Senate: `senatestockwatcher.com/api/transactions`
- These are more reliable for this data anyway — dedicated sources
- Finnhub still handles everything else: live prices, news, IPO calendar

### API Cost & Free Tier
- Finnhub free tier: 60 REST calls/min, 50 WebSocket symbols
  - App uses 3 WebSocket symbols (AAPL, TSLA, RIVN) — well within limits
  - Congressional data no longer goes through Finnhub
  - News calls only fire when anomalies are detected
  - IPO: 1 call/day
  - Redis rate limiter enforces the 60/min cap with a buffer of 55
- House/Senate Stock Watcher: free, no key, no rate limits documented
- All infrastructure (TimescaleDB, Redis, Kafka, Docker) runs locally — zero cost

---

## Project Layout

```
Watchtower/
├── docker-compose.yml           # TimescaleDB, Redis, Kafka + Zookeeper
├── .env.example                 # API keys and DSNs (never committed)
├── .env                         # Your actual keys (gitignored)
├── Makefile                     # dev shortcuts: make up, make down, make logs
├── PLANNING.md                  # This file
├── db/
│   └── migrations/
│       └── 001_init.sql         # All 7 tables + hypertable creation
├── backend/                     # Go service
│   ├── Dockerfile
│   ├── go.mod
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          # Entrypoint — wires all subsystems
│   └── internal/
│       ├── config/              # Env loading (godotenv)
│       ├── db/                  # pgx pool + TimescaleDB helpers
│       ├── redis/               # go-redis client + rate-limit middleware
│       ├── finnhub/             # WebSocket client + REST wrappers (news, IPO)
│       ├── congress/            # House/Senate Stock Watcher polling
│       ├── kafka/               # Sarama producer + consumer
│       ├── anomaly/             # Volume-spike ring-buffer detector
│       └── handlers/            # Gin HTTP + WebSocket handlers
├── sentiment-worker/            # TypeScript / Node.js microservice
│   ├── Dockerfile
│   ├── package.json             # confluent-kafka-node, sentiment, node-fetch
│   └── src/
│       └── index.ts             # Kafka consumer → AFINN scoring → result publish
└── frontend/                    # SvelteKit app
    ├── Dockerfile
    ├── package.json
    └── src/
        ├── routes/
        │   ├── +page.svelte     # Live tick dashboard + anomaly sidebar
        │   ├── congress/
        │   │   └── +page.svelte # Congressional trading heatmap
        │   └── ipo/
        │       └── +page.svelte # IPO risk rater grid
        └── lib/
            ├── ws.ts            # WebSocket Svelte store
            └── charts.ts        # Lightweight Charts wrappers
```

---

## Data Flow

```
Finnhub WebSocket (AAPL, TSLA, RIVN)
        │
        ▼
   Go Backend (Gin)
   ┌────────────────────────────────┐
   │  Finnhub WS Client (goroutine) │
   │  → writes to market_ticks      │
   │  → broadcasts to WS hub        │
   │  → feeds Anomaly Detector      │
   └────────────────────────────────┘
        │                    │
        ▼                    ▼
  TimescaleDB          Browser Clients
  (market_ticks        (SvelteKit via WS)
   hypertable)
        
   Anomaly Detector (goroutine, ring buffer per symbol)
        │  if volume > 3x 10-tick avg
        ▼
   Kafka topic: market-anomalies
   { symbol, time, trigger_volume, avg_volume }
        │
        ▼
   TypeScript Sentiment Worker (Node.js)
   → fetches Finnhub news for symbol (last 24h)
   → scores headlines with AFINN sentiment
   → averages compound scores
   → writes to sentiment_analysis table
        │
        ▼
   Kafka topic: sentiment-results
        │
        ▼
   Go Kafka Consumer
   → broadcasts sentiment payload over WS hub
        │
        ▼
   SvelteKit dashboard updates in real time
   (anomaly card + sentiment score + top headline)


   Separate cron jobs (Go):
   - Every 6h: House + Senate Stock Watcher → congressional_trades table
   - Daily: Finnhub IPO calendar → ipo_evaluations table (with risk scoring)
```

---

## Database Schema (TimescaleDB / PostgreSQL)

```sql
-- Run: CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       VARCHAR NOT NULL UNIQUE,
    password_hash VARCHAR NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE watchlists (
    id          SERIAL PRIMARY KEY,
    user_id     UUID REFERENCES users(id),
    symbol      VARCHAR NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Hypertable — partitioned by time
CREATE TABLE market_ticks (
    time        TIMESTAMPTZ NOT NULL,
    symbol      VARCHAR NOT NULL,
    price       DECIMAL NOT NULL,
    volume      INT NOT NULL
);
SELECT create_hypertable('market_ticks', 'time');

CREATE TABLE anomaly_events (
    id              SERIAL PRIMARY KEY,
    time            TIMESTAMPTZ NOT NULL,
    symbol          VARCHAR NOT NULL,
    anomaly_type    VARCHAR NOT NULL,
    trigger_value   DECIMAL NOT NULL
);

CREATE TABLE sentiment_analysis (
    id              SERIAL PRIMARY KEY,
    event_id        INT REFERENCES anomaly_events(id),
    symbol          VARCHAR NOT NULL,
    timestamp       TIMESTAMPTZ DEFAULT NOW(),
    sentiment_score DECIMAL NOT NULL,
    article_count   INT NOT NULL,
    top_headline    TEXT
);

CREATE TABLE congressional_trades (
    id                  SERIAL PRIMARY KEY,
    symbol              VARCHAR NOT NULL,
    representative_name VARCHAR NOT NULL,
    transaction_date    DATE NOT NULL,
    transaction_type    VARCHAR CHECK (transaction_type IN ('BUY', 'SELL')),
    amount_range        VARCHAR,
    UNIQUE(symbol, representative_name, transaction_date)
);

CREATE TABLE ipo_evaluations (
    id              SERIAL PRIMARY KEY,
    symbol          VARCHAR NOT NULL,
    company_name    VARCHAR NOT NULL,
    expected_date   DATE,
    risk_score      INT,
    sector          VARCHAR
);
```

---

## Implementation Phases

### Phase 1 — Infrastructure & Ingestion
**Goal:** Docker stack running, live prices flowing into the DB and onto a chart.

Tasks:
- [ ] `docker-compose.yml` — TimescaleDB, Redis, Kafka, Zookeeper with health checks and shared network
- [ ] `.env.example` and `Makefile`
- [ ] `db/migrations/001_init.sql` — all 7 tables + hypertable
- [ ] Go module init — Gin, pgx/v5, gorilla/websocket, go-redis, IBM/sarama, godotenv
- [ ] Finnhub WebSocket client in Go (AAPL, TSLA, RIVN) → channel fan-out
- [ ] Batch tick writer to `market_ticks` via pgx `CopyFrom`
- [ ] Go WebSocket hub — broadcasts ticks to all connected browser clients
- [ ] SvelteKit scaffold (TypeScript, Tailwind, Lightweight Charts)
- [ ] WS store + live chart rendering (3-column grid, one chart per symbol)

**Done when:** You can run `make up`, start the Go server, open the browser, and see live prices updating on charts.

---

### Phase 2 — Analytics & Caching
**Goal:** Congressional trades and IPO data visible in the UI, rate limiting in place.

Tasks:
- [ ] Redis sliding-window rate limiter middleware (Go) — 55 calls/min cap on Finnhub REST
- [ ] Congressional trades cron (every 6h) — polls House + Senate Stock Watcher APIs, upserts to DB
- [ ] `GET /api/congress/:symbol` endpoint
- [ ] IPO calendar cron (daily) — polls Finnhub, applies risk scoring, upserts to DB
- [ ] `GET /api/ipo` endpoint
- [ ] SvelteKit: Congressional heatmap page (trades overlaid on price chart)
- [ ] SvelteKit: IPO grid page (sortable, color-coded by risk score)

**Done when:** Congressional and IPO pages load with real data, and the rate limiter protects the Finnhub key.

---

### Phase 3 — The Event-Driven Brain
**Goal:** Volume spike → Kafka → sentiment → dashboard update, end-to-end, automatically.

Tasks:
- [ ] Ring-buffer anomaly detector goroutine in Go (10-tick rolling avg, 3x threshold)
- [ ] Write anomaly to `anomaly_events` table on detection
- [ ] Sarama Kafka producer publishing to `market-anomalies` topic
- [ ] TypeScript sentiment worker (Node.js):
  - Kafka consumer on `market-anomalies`
  - Fetch 24h of Finnhub company news for the symbol
  - Score all headlines with `sentiment` npm package
  - Average compound scores
  - Write to `sentiment_analysis` table
  - Publish to `sentiment-results` Kafka topic
- [ ] Go Kafka consumer on `sentiment-results` → WS hub broadcast
- [ ] SvelteKit: anomaly event sidebar (live list of spikes)
- [ ] SvelteKit: sentiment card per anomaly (score gauge, article count, top headline)

**Done when:** A real or simulated volume spike fires, and within ~5 seconds a sentiment card appears on the dashboard with no manual action.

---

## Environment Variables

```env
# Finnhub
FINNHUB_API_KEY=your_key_here

# TimescaleDB
DATABASE_URL=postgres://watchtower:watchtower@localhost:5432/watchtower

# Redis
REDIS_URL=redis://localhost:6379

# Kafka
KAFKA_BROKERS=localhost:9092

# Server
SERVER_PORT=8080
```

Get your free Finnhub API key at: https://finnhub.io (no credit card required for free tier)

---

## Go Dependencies

```
github.com/gin-gonic/gin
github.com/gorilla/websocket
github.com/jackc/pgx/v5
github.com/redis/go-redis/v9
github.com/IBM/sarama
github.com/joho/godotenv
```

## TypeScript Sentiment Worker Dependencies

```
kafkajs          (Kafka client — cleaner API than confluent-kafka-node)
sentiment        (AFINN-based NLP scoring)
node-fetch       (HTTP calls to Finnhub)
pg               (PostgreSQL client)
```

> Note: Using `kafkajs` instead of `confluent-kafka-node` in the worker — pure JS, no native bindings, easier Docker build.

## Frontend Dependencies

```
lightweight-charts    (HTML5 Canvas charting)
tailwindcss
```

---

## Infrastructure Notes

- Kafka version: `confluentinc/cp-kafka:7.6` with Zookeeper (not KRaft) for simplicity
- TimescaleDB version: `timescale/timescaledb:latest-pg15`
- Redis version: `redis:7-alpine`
- All containers share `watchtower_net` bridge network
- `market_ticks` hypertable: always query with a `time` WHERE clause to leverage chunk pruning
- Congressional trades: upserted with a `UNIQUE` constraint on `(symbol, representative_name, transaction_date)` to avoid duplicates on re-polling
- Anomaly threshold: `volume > 3.0 * avg(last 10 ticks)` — tunable constant

---

## Security Notes (workspace rules apply to all generated code)

- No API keys, tokens, or secrets are ever logged — only correlation IDs and sanitized event summaries
- `.env` is gitignored; `.env.example` is committed with placeholder values
- All SQL uses parameterized queries (pgx named args / positional `$1, $2`)
- User passwords stored as hashes only (bcrypt)
- WebSocket connections do not forward auth headers to upstream services
