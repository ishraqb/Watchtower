import pg from 'pg';
import { config } from './config.js';

const { Pool } = pg;

const pool = new Pool({ connectionString: config.databaseUrl });

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
