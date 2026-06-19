// Package redisstream is the production stand-in for Kafka. Free hosting doesn't
// give us a managed Kafka, but it does give us Redis, and Redis Streams cover
// what we need here: an append-only log with consumer groups and replay, so a
// result isn't lost if a service restarts. Same interfaces as the kafka package.
package redisstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ishraqb/Watchtower/backend/internal/broker"
)

const (
	anomalyStream   = "watchtower:anomalies"
	sentimentStream = "watchtower:sentiment"
	// Trim the streams so they can't grow forever on a long-running instance.
	maxStreamLen = 1000
)

// Publisher writes anomalies onto a Redis Stream. If wakeURL is set it also
// nudges the (possibly asleep) worker so it spins up and drains the stream.
type Publisher struct {
	rdb     *redis.Client
	wakeURL string
}

// NewPublisher builds a publisher. wakeURL is optional — pass "" to disable the
// wake nudge (e.g. when the worker is always running).
func NewPublisher(rdb *redis.Client, wakeURL string) *Publisher {
	return &Publisher{rdb: rdb, wakeURL: strings.TrimSpace(wakeURL)}
}

// PublishAnomaly appends the anomaly to the stream and wakes the worker.
func (p *Publisher) PublishAnomaly(msg broker.AnomalyMessage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("redisstream: marshal anomaly: %w", err)
	}
	err = p.rdb.XAdd(context.Background(), &redis.XAddArgs{
		Stream: anomalyStream,
		MaxLen: maxStreamLen,
		Approx: true, // ~MAXLEN is much cheaper than exact trimming
		Values: map[string]interface{}{"payload": payload},
	}).Err()
	if err != nil {
		return fmt.Errorf("redisstream: xadd anomaly: %w", err)
	}
	p.wake()
	return nil
}

// wake fires a fire-and-forget GET at the worker's URL so a spun-down free-tier
// instance comes back up and processes the anomaly we just queued.
func (p *Publisher) wake() {
	if p.wakeURL == "" {
		return
	}
	go func() {
		// wakeURL is an operator-set env value (the worker's own URL), never user
		// input, so this is not an SSRF vector.
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest(http.MethodGet, p.wakeURL, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			return // worker may be mid-cold-start; the stream keeps the message
		}
		_ = resp.Body.Close()
	}()
}

// Close is a no-op; the shared redis client is owned (and closed) by main.
func (p *Publisher) Close() error { return nil }

// Consumer reads sentiment results off a Redis Stream via a consumer group, so
// nothing is missed across backend restarts.
type Consumer struct {
	rdb     *redis.Client
	group   string
	handler broker.SentimentHandler
}

// NewConsumer builds a sentiment consumer for the given group.
func NewConsumer(rdb *redis.Client, group string, handler broker.SentimentHandler) *Consumer {
	return &Consumer{rdb: rdb, group: group, handler: handler}
}

// Run blocks reading the sentiment stream until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	// Create the group (and the stream, via MKSTREAM) if it isn't there yet.
	// "$" = only deliver entries added after the group is created.
	if err := c.rdb.XGroupCreateMkStream(ctx, sentimentStream, c.group, "$").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		log.Printf("redisstream: create group: %v", err)
	}

	for {
		if ctx.Err() != nil {
			return
		}
		res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: "backend",
			Streams:  []string{sentimentStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				continue // just a block timeout with nothing new
			}
			log.Printf("redisstream: read group: %v", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range res {
			for _, m := range stream.Messages {
				raw, _ := m.Values["payload"].(string)
				var msg broker.SentimentMessage
				if err := json.Unmarshal([]byte(raw), &msg); err != nil {
					log.Printf("redisstream: bad message: %v", err)
				} else {
					c.handler(msg)
				}
				c.rdb.XAck(ctx, sentimentStream, c.group, m.ID)
			}
		}
	}
}

// Close is a no-op; the shared redis client is owned (and closed) by main.
func (c *Consumer) Close() error { return nil }
