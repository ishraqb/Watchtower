import { config } from '../config.js';
import { KafkaBroker } from './kafka.js';
import { RedisStreamBroker } from './redis.js';
import type { Broker } from './types.js';

// Picks the broker from config: Kafka for local/dev, Redis Streams when
// BROKER=redis (the free hosted build).
export function createBroker(): Broker {
	if (config.broker === 'redis') {
		return new RedisStreamBroker(config.redisUrl);
	}
	return new KafkaBroker(config.kafkaBrokers);
}

export type { Broker, AnomalyMessage, SentimentMessage } from './types.js';
