// Sentiment worker: reads volume anomalies off the broker, pulls the company
// news for that ticker, scores the headlines, and publishes the result back so
// the backend can show it on the live feed. Kept as a separate service so the
// scoring work never blocks the main tick ingestion path.
//
// The broker is pluggable (Kafka locally, Redis Streams on free hosting) and
// there's a tiny HTTP server so the free-tier host can health-check and wake it.
import http from 'node:http';
import Sentiment from 'sentiment';
import { config } from './config.js';
import { createBroker, type AnomalyMessage, type Broker, type SentimentMessage } from './broker/index.js';
import { fetchCompanyNews } from './finnhub.js';
import { insertSentiment, closePool } from './db.js';

const sentiment = new Sentiment();
const broker: Broker = createBroker();

/**
 * Scores all headlines for an anomaly's symbol and republishes the result.
 * AFINN comparative scores are roughly bounded; we clamp the average to
 * [-1, 1] to match the VADER-style compound scale used downstream.
 */
async function processAnomaly(msg: AnomalyMessage): Promise<void> {
	const articles = await fetchCompanyNews(msg.symbol, 1);

	let scoreSum = 0;
	let topHeadline = '';
	let topAbs = -1;

	for (const article of articles) {
		const headline = article.headline ?? '';
		if (!headline) continue;
		const { comparative } = sentiment.analyze(headline);
		scoreSum += comparative;
		if (Math.abs(comparative) > topAbs) {
			topAbs = Math.abs(comparative);
			topHeadline = headline;
		}
	}

	const count = articles.length;
	const avg = count > 0 ? scoreSum / count : 0;
	const clamped = Math.max(-1, Math.min(1, avg));

	await insertSentiment({
		eventId: msg.event_id,
		symbol: msg.symbol,
		score: clamped,
		articleCount: count,
		topHeadline
	});

	const result: SentimentMessage = {
		event_id: msg.event_id,
		symbol: msg.symbol,
		sentiment_score: clamped,
		article_count: count,
		top_headline: topHeadline
	};
	await broker.publishSentiment(result);

	console.log(
		`sentiment: ${msg.symbol} event ${msg.event_id} -> score ${clamped.toFixed(3)} over ${count} articles`
	);
}

/**
 * Tiny HTTP server. On the free tier a service needs to listen on a port, and
 * the backend pings /wake to spin us back up when an anomaly fires. The actual
 * processing is the broker loop that's already running - /wake just gets us out
 * of a spun-down state so that loop can drain the stream.
 */
function startHealthServer(): void {
	const server = http.createServer((req, res) => {
		if (req.url === '/health' || req.url === '/wake') {
			res.writeHead(200, { 'Content-Type': 'application/json' });
			res.end(JSON.stringify({ status: 'ok' }));
			return;
		}
		res.writeHead(404);
		res.end();
	});
	server.listen(config.port, () => {
		console.log(`sentiment-worker: health server listening on :${config.port}`);
	});
}

async function main(): Promise<void> {
	startHealthServer();
	await broker.start(processAnomaly);
	console.log(`sentiment-worker: started (broker=${config.broker})`);
}

async function shutdown(): Promise<void> {
	console.log('sentiment-worker: shutting down');
	try {
		await broker.shutdown();
		await closePool();
	} finally {
		process.exit(0);
	}
}

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

main().catch((err) => {
	console.error('sentiment-worker: fatal:', (err as Error).message);
	process.exit(1);
});
