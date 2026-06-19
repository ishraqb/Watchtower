// Package broker is the seam between the part of the app that detects anomalies
// and the worker that scores their sentiment. We run Kafka locally (it's what
// the project is built around) but Kafka isn't available on free hosting, so the
// deployed build uses Redis Streams instead. Both live behind these interfaces
// and main.go picks one at startup based on the BROKER env var.
package broker

import (
	"context"
	"time"
)

// AnomalyMessage is what we emit when the detector spots a volume spike.
type AnomalyMessage struct {
	EventID       int       `json:"event_id"`
	Symbol        string    `json:"symbol"`
	Time          time.Time `json:"time"`
	TriggerVolume int       `json:"trigger_volume"`
	AvgVolume     float64   `json:"avg_volume"`
}

// SentimentMessage is the worker's reply once it has read and scored the news.
type SentimentMessage struct {
	EventID        int     `json:"event_id"`
	Symbol         string  `json:"symbol"`
	SentimentScore float64 `json:"sentiment_score"`
	ArticleCount   int     `json:"article_count"`
	TopHeadline    string  `json:"top_headline"`
}

// SentimentHandler is called for each sentiment result that comes back.
type SentimentHandler func(SentimentMessage)

// AnomalyPublisher sends detected anomalies onward for the worker to pick up.
type AnomalyPublisher interface {
	PublishAnomaly(AnomalyMessage) error
	Close() error
}

// SentimentConsumer streams scored results back and feeds each one to the
// handler it was built with, until ctx is cancelled.
type SentimentConsumer interface {
	Run(ctx context.Context)
	Close() error
}
