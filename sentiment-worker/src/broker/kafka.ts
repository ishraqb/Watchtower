import { Kafka, type Consumer, type Producer } from 'kafkajs';
import type { AnomalyHandler, Broker, SentimentMessage } from './types.js';

const TOPIC_ANOMALIES = 'market-anomalies';
const TOPIC_SENTIMENT = 'sentiment-results';

// KafkaBroker is the local/dev path - the real Kafka the project is built on.
export class KafkaBroker implements Broker {
	private readonly consumer: Consumer;
	private readonly producer: Producer;

	constructor(brokers: string[]) {
		const kafka = new Kafka({ clientId: 'sentiment-worker', brokers });
		this.consumer = kafka.consumer({ groupId: 'sentiment-worker-group' });
		this.producer = kafka.producer();
	}

	async start(onAnomaly: AnomalyHandler): Promise<void> {
		await this.producer.connect();
		await this.consumer.connect();
		await this.consumer.subscribe({ topic: TOPIC_ANOMALIES, fromBeginning: false });
		await this.consumer.run({
			eachMessage: async ({ message }) => {
				if (!message.value) return;
				try {
					await onAnomaly(JSON.parse(message.value.toString()));
				} catch (err) {
					// Don't leak payloads (could contain secrets) - message only.
					console.error('worker: failed to process message:', (err as Error).message);
				}
			}
		});
	}

	async publishSentiment(msg: SentimentMessage): Promise<void> {
		await this.producer.send({
			topic: TOPIC_SENTIMENT,
			messages: [{ key: msg.symbol, value: JSON.stringify(msg) }]
		});
	}

	async shutdown(): Promise<void> {
		await this.consumer.disconnect();
		await this.producer.disconnect();
	}
}
