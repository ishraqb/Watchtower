import Redis from 'ioredis';
import type { AnomalyHandler, Broker, SentimentMessage } from './types.js';

const ANOMALY_STREAM = 'watchtower:anomalies';
const SENTIMENT_STREAM = 'watchtower:sentiment';
const GROUP = 'sentiment-worker';
const CONSUMER = 'worker';
const MAX_LEN = 1000;

// RedisStreamBroker is the production path. Free hosting has no managed Kafka,
// so we use Redis Streams instead. The consumer group means anomalies that piled
// up while this worker was spun down (free tier) still get processed on wake.
export class RedisStreamBroker implements Broker {
	private readonly redis: Redis; // XADD / XACK / XGROUP
	private readonly blocking: Redis; // dedicated connection for blocking reads
	private running = false;

	constructor(url: string) {
		this.redis = new Redis(url);
		this.blocking = new Redis(url);
	}

	async start(onAnomaly: AnomalyHandler): Promise<void> {
		// Create the group from the very start of the stream ('0') so messages
		// added before/while the worker was asleep aren't skipped. BUSYGROUP just
		// means it already exists, in which case it resumes where it left off.
		try {
			await this.redis.xgroup('CREATE', ANOMALY_STREAM, GROUP, '0', 'MKSTREAM');
		} catch (err) {
			if (!String((err as Error).message).includes('BUSYGROUP')) throw err;
		}
		this.running = true;
		void this.loop(onAnomaly);
	}

	private async loop(onAnomaly: AnomalyHandler): Promise<void> {
		while (this.running) {
			try {
				// Cast: ioredis's xreadgroup overloads don't model the reply shape well.
				const res = (await this.blocking.xreadgroup(
					'GROUP',
					GROUP,
					CONSUMER,
					'COUNT',
					10,
					'BLOCK',
					5000,
					'STREAMS',
					ANOMALY_STREAM,
					'>'
				)) as [string, [string, string[]][]][] | null;

				if (!res) continue; // block timed out with nothing new

				for (const [, entries] of res) {
					for (const [id, fields] of entries) {
						const payload = fieldValue(fields, 'payload');
						if (payload) {
							try {
								await onAnomaly(JSON.parse(payload));
							} catch (err) {
								console.error('worker: failed to process anomaly:', (err as Error).message);
							}
						}
						await this.redis.xack(ANOMALY_STREAM, GROUP, id);
					}
				}
			} catch (err) {
				console.error('worker: stream read error:', (err as Error).message);
				await new Promise((resolve) => setTimeout(resolve, 1000));
			}
		}
	}

	async publishSentiment(msg: SentimentMessage): Promise<void> {
		await this.redis.xadd(
			SENTIMENT_STREAM,
			'MAXLEN',
			'~',
			MAX_LEN,
			'*',
			'payload',
			JSON.stringify(msg)
		);
	}

	async shutdown(): Promise<void> {
		this.running = false;
		this.redis.disconnect();
		this.blocking.disconnect();
	}
}

// Redis Stream entries are flat [field, value, field, value, ...] arrays.
function fieldValue(fields: string[], key: string): string | undefined {
	for (let i = 0; i < fields.length - 1; i += 2) {
		if (fields[i] === key) return fields[i + 1];
	}
	return undefined;
}
