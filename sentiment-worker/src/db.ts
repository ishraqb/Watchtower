import pg from 'pg';
import { config } from './config.js';

const { Pool } = pg;

// node-postgres doesn't reliably turn on TLS from the connection string's
// sslmode param, so enable it explicitly for managed Postgres (e.g. Neon) that
// requires it. Local Docker Postgres uses sslmode=disable and stays plaintext.
const needsSsl = /sslmode=(require|verify-ca|verify-full)|neon\.tech/.test(config.databaseUrl);
const pool = new Pool({
	connectionString: config.databaseUrl,
	...(needsSsl ? { ssl: { rejectUnauthorized: true } } : {})
});

export interface SentimentRecord {
	eventId: number;
	symbol: string;
	score: number;
	articleCount: number;
	topHeadline: string;
}

/**
 * Inserts a sentiment result. Uses a parameterized query so user/event-derived
 * values are bound as data, never interpolated into SQL (injection-safe).
 */
export async function insertSentiment(rec: SentimentRecord): Promise<void> {
	await pool.query(
		`INSERT INTO sentiment_analysis
		   (event_id, symbol, sentiment_score, article_count, top_headline)
		 VALUES ($1, $2, $3, $4, $5)`,
		[rec.eventId, rec.symbol, rec.score, rec.articleCount, rec.topHeadline]
	);
}

export async function closePool(): Promise<void> {
	await pool.end();
}
