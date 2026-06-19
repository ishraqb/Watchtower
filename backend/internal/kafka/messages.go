// Package kafka is the local/dev message broker between the Go backend and the
// TS sentiment worker: the API publishes anomalies, the worker publishes
// sentiment back. The message shapes themselves live in the broker package so
// the Redis Streams implementation can share them.
package kafka

// Topic names shared across the producer, the Go consumer, and the TS worker.
const (
	TopicAnomalies = "market-anomalies"
	TopicSentiment = "sentiment-results"
)
