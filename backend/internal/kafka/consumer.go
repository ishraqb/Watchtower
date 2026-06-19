package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/IBM/sarama"

	"github.com/ishraqb/Watchtower/backend/internal/broker"
)

// Consumer reads sentiment results and forwards them to a handler.
type Consumer struct {
	group   sarama.ConsumerGroup
	handler broker.SentimentHandler
}

// NewConsumer joins a consumer group on the given brokers.
func NewConsumer(brokers string, groupID string, handler broker.SentimentHandler) (*Consumer, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	cfg.Consumer.Return.Errors = true

	group, err := sarama.NewConsumerGroup(strings.Split(brokers, ","), groupID, cfg)
	if err != nil {
		return nil, err
	}
	return &Consumer{group: group, handler: handler}, nil
}

// Run consumes TopicSentiment until the context is cancelled, rejoining the
// group automatically after a rebalance.
func (c *Consumer) Run(ctx context.Context) {
	go func() {
		for err := range c.group.Errors() {
			log.Printf("kafka consumer: %v", err)
		}
	}()

	gh := &groupHandler{handler: c.handler}
	for {
		if err := c.group.Consume(ctx, []string{TopicSentiment}, gh); err != nil {
			log.Printf("kafka consumer: consume error: %v", err)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// Close shuts down the consumer group.
func (c *Consumer) Close() error {
	return c.group.Close()
}

// groupHandler implements sarama.ConsumerGroupHandler.
type groupHandler struct {
	handler broker.SentimentHandler
}

func (h *groupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *groupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *groupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			var sentiment broker.SentimentMessage
			if err := json.Unmarshal(msg.Value, &sentiment); err != nil {
				log.Printf("kafka consumer: bad message: %v", err)
			} else {
				h.handler(sentiment)
			}
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}
