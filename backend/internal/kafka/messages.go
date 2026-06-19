package kafka

import "time"

// Topic names shared across the producer, the Go consumer, and the TS worker.
const (
	TopicAnomalies = "market-anomalies"
	TopicSentiment = "sentiment-results"
)

// AnomalyMessage is published to TopicAnomalies when a volume spike is detected.
type AnomalyMessage struct {
	EventID       int       `json:"event_id"`
	Symbol        string    `json:"symbol"`
	Time          time.Time `json:"time"`
	TriggerVolume int       `json:"trigger_volume"`
	AvgVolume     float64   `json:"avg_volume"`
}

// SentimentMessage is published to TopicSentiment by the TS worker and consumed
// by the Go backend, which forwards it to browser clients.
type SentimentMessage struct {
	EventID        int     `json:"event_id"`
	Symbol         string  `json:"symbol"`
	SentimentScore float64 `json:"sentiment_score"`
	ArticleCount   int     `json:"article_count"`
	TopHeadline    string  `json:"top_headline"`
}
