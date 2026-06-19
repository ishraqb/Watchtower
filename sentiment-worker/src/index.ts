import { Kafka, type Consumer, type Producer } from 'kafkajs';
import Sentiment from 'sentiment';
import { config, TOPIC_ANOMALIES, TOPIC_SENTIMENT } from './config.js';
import { fetchCompanyNews } from './finnhub.js';
import { insertSentiment, closePool } from './db.js';

interface AnomalyMessage {
	event_id: number;
	symbol: string;
	time: string;
	trigger_volume: number;
	avg_volume: number;
}

interface SentimentMessage {
	event_id: number;
	symbol: string;
	sentiment_score: number;
	article_count: number;
	top_headline: string;
}

const sentiment = new Sentiment();

const kafka = new Kafka({ clientId: 'sentiment-worker', brokers: config.kafkaBrokers });
const consumer: Consumer = kafka.consumer({ groupId: 'sentiment-worker-group' });
const producer: Producer = kafka.producer();

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

	await producer.send({
		topic: TOPIC_SENTIMENT,
		messages: [{ key: msg.symbol, value: JSON.stringify(result) }]
	});

	console.log(
		`sentiment: ${msg.symbol} event ${msg.event_id} -> score ${clamped.toFixed(3)} over ${count} articles`
	);
}

async function main(): Promise<void> {
	await producer.connect();
	await consumer.connect();
	await consumer.subscribe({ topic: TOPIC_ANOMALIES, fromBeginning: false });

	console.log('sentiment-worker: consuming', TOPIC_ANOMALIES);

	await consumer.run({
		eachMessage: async ({ message }) => {
			if (!message.value) return;
			try {
				const anomaly = JSON.parse(message.value.toString()) as AnomalyMessage;
				await processAnomaly(anomaly);
			} catch (err) {
				// Log the failure without leaking payloads that could contain secrets.
				console.error('sentiment-worker: failed to process message:', (err as Error).message);
			}
		}
	});
}

async function shutdown(): Promise<void> {
	console.log('sentiment-worker: shutting down');
	try {
		await consumer.disconnect();
		await producer.disconnect();
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
