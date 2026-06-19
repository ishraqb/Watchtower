// Shared message shapes and the broker contract. The worker doesn't care
// whether anomalies arrive over Kafka (local/dev) or Redis Streams (the free
// hosted build) - it just needs anomalies in and sentiment out.

export interface AnomalyMessage {
	event_id: number;
	symbol: string;
	time: string;
	trigger_volume: number;
	avg_volume: number;
}

export interface SentimentMessage {
	event_id: number;
	symbol: string;
	sentiment_score: number;
	article_count: number;
	top_headline: string;
}

export type AnomalyHandler = (msg: AnomalyMessage) => Promise<void>;

export interface Broker {
	// start begins consuming anomalies, calling onAnomaly for each one.
	start(onAnomaly: AnomalyHandler): Promise<void>;
	// publishSentiment sends a scored result back for the backend to pick up.
	publishSentiment(msg: SentimentMessage): Promise<void>;
	shutdown(): Promise<void>;
}
