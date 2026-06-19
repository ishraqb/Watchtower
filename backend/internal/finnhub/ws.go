package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

// DefaultSymbols are the tickers Watchtower subscribes to on the free tier.
var DefaultSymbols = []string{"AAPL", "TSLA", "RIVN"}

const wsURL = "wss://ws.finnhub.io"

// tradeMessage mirrors the Finnhub trade payload shape.
type tradeMessage struct {
	Type string `json:"type"`
	Data []struct {
		Symbol string  `json:"s"`
		Price  float64 `json:"p"`
		Volume float64 `json:"v"`
		TimeMS int64   `json:"t"`
	} `json:"data"`
}

type subscribeMessage struct {
	Type   string `json:"type"`
	Symbol string `json:"symbol"`
}

// Client streams live trades from Finnhub onto an outbound channel.
type Client struct {
	apiKey  string
	symbols []string
	Ticks   chan db.Tick
}

// NewClient builds a Finnhub WS client. The Ticks channel fans ticks out to consumers.
func NewClient(apiKey string, symbols []string) *Client {
	return &Client{
		apiKey:  apiKey,
		symbols: symbols,
		Ticks:   make(chan db.Tick, 1024),
	}
}

// Run connects, subscribes, and pumps trades until the context is cancelled.
// It reconnects automatically on transient errors with a short backoff.
func (c *Client) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.connectAndStream(ctx); err != nil {
			// Never log the API key; it is only ever placed in the dial URL.
			log.Printf("finnhub: stream ended (%v), reconnecting in 5s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (c *Client) connectAndStream(ctx context.Context) error {
	dialURL := fmt.Sprintf("%s?token=%s", wsURL, c.apiKey)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, dialURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	for _, sym := range c.symbols {
		if err := conn.WriteJSON(subscribeMessage{Type: "subscribe", Symbol: sym}); err != nil {
			return fmt.Errorf("subscribe %s: %w", sym, err)
		}
	}
	log.Printf("finnhub: connected and subscribed to %v", c.symbols)

	// Close the connection when the context is cancelled to unblock ReadMessage.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var msg tradeMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue // ping/pong or unknown frame
		}
		if msg.Type != "trade" {
			continue
		}

		for _, t := range msg.Data {
			tick := db.Tick{
				Time:   time.UnixMilli(t.TimeMS),
				Symbol: t.Symbol,
				Price:  t.Price,
				Volume: int(t.Volume),
			}
			select {
			case c.Ticks <- tick:
			default:
				// Drop if downstream is saturated rather than stalling the reader.
			}
		}
	}
}
