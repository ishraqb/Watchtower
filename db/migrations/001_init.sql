-- Watchtower initial schema
-- Runs automatically on first container start via docker-entrypoint-initdb.d
-- locally, and via `make migrate` against a plain Postgres (e.g. Neon) in prod.

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR NOT NULL UNIQUE,
    password_hash VARCHAR NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS watchlists (
    id         SERIAL PRIMARY KEY,
    user_id    UUID REFERENCES users(id),
    symbol     VARCHAR NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- High-frequency tick data. Locally this becomes a TimescaleDB hypertable
-- (partitioned by time); on a plain Postgres that lacks the extension it's just
-- a regular table. Either way the columns, index, and queries are identical.
CREATE TABLE IF NOT EXISTS market_ticks (
    time   TIMESTAMPTZ NOT NULL,
    symbol VARCHAR NOT NULL,
    price  DECIMAL NOT NULL,
    volume INT NOT NULL
);

-- Only reach for TimescaleDB when it's actually installed, so this same file
-- runs cleanly against vanilla Postgres (Neon) where the extension isn't offered.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;
        PERFORM create_hypertable('market_ticks', 'time', if_not_exists => TRUE);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_market_ticks_symbol_time ON market_ticks (symbol, time DESC);

CREATE TABLE IF NOT EXISTS anomaly_events (
    id            SERIAL PRIMARY KEY,
    time          TIMESTAMPTZ NOT NULL,
    symbol        VARCHAR NOT NULL,
    anomaly_type  VARCHAR NOT NULL,
    trigger_value DECIMAL NOT NULL
);

CREATE TABLE IF NOT EXISTS sentiment_analysis (
    id              SERIAL PRIMARY KEY,
    event_id        INT REFERENCES anomaly_events(id),
    symbol          VARCHAR NOT NULL,
    timestamp       TIMESTAMPTZ DEFAULT NOW(),
    sentiment_score DECIMAL NOT NULL,
    article_count   INT NOT NULL,
    top_headline    TEXT
);

CREATE TABLE IF NOT EXISTS congressional_trades (
    id                  SERIAL PRIMARY KEY,
    symbol              VARCHAR NOT NULL,
    representative_name VARCHAR NOT NULL,
    transaction_date    DATE NOT NULL,
    transaction_type    VARCHAR CHECK (transaction_type IN ('BUY', 'SELL')),
    amount_range        VARCHAR,
    UNIQUE (symbol, representative_name, transaction_date)
);

CREATE TABLE IF NOT EXISTS ipo_evaluations (
    id            SERIAL PRIMARY KEY,
    symbol        VARCHAR NOT NULL,
    company_name  VARCHAR NOT NULL,
    expected_date DATE,
    risk_score    INT,
    sector        VARCHAR,
    exchange      VARCHAR,
    price_range   VARCHAR,
    shares_value  BIGINT,
    UNIQUE (symbol, expected_date)
);
