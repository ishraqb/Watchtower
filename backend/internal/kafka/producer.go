package kafka

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/IBM/sarama"
)

// Producer publishes messages to Kafka topics.
type Producer struct {
	sync sarama.SyncProducer
}

// NewProducer connects a synchronous producer to the given brokers.
func NewProducer(brokers string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 5
	cfg.Producer.Return.Successes = true

	brokerList := strings.Split(brokers, ",")
	sp, err := sarama.NewSyncProducer(brokerList, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka: new producer: %w", err)
	}
	return &Producer{sync: sp}, nil
}

// PublishAnomaly serializes and sends an anomaly to TopicAnomalies, keyed by symbol
// so that all events for one symbol land on the same partition (ordered).
func (p *Producer) PublishAnomaly(msg AnomalyMessage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("kafka: marshal anomaly: %w", err)
	}

	_, _, err = p.sync.SendMessage(&sarama.ProducerMessage{
		Topic: TopicAnomalies,
		Key:   sarama.StringEncoder(msg.Symbol),
		Value: sarama.ByteEncoder(payload),
	})
	if err != nil {
		return fmt.Errorf("kafka: send anomaly: %w", err)
	}
	return nil
}

// Close flushes and shuts down the producer.
func (p *Producer) Close() error {
	return p.sync.Close()
}
