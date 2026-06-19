-- Watchtower initial schema
-- Runs automatically on first container start via docker-entrypoint-initdb.d

CREATE EXTENSION IF NOT EXISTS timescaledb;

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

-- High-frequency tick data — partitioned by time via TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS market_ticks (
    time   TIMESTAMPTZ NOT NULL,
    symbol VARCHAR NOT NULL,
    price  DECIMAL NOT NULL,
    volume INT NOT NULL
);
SELECT create_hypertable('market_ticks', 'time', if_not_exists => TRUE);
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
