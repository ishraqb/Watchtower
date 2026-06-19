// Package finnhub is everything that talks to Finnhub: the live trade
// websocket (this file), one-off quotes, and the IPO calendar poller.
package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

// DefaultSymbols are the tickers Watchtower subscribes to on the free tier.
var DefaultSymbols = []string{"AAPL", "TSLA", "RIVN"}

// maxSymbols caps total WS subscriptions to stay within the Finnhub free-tier
// limit (50 symbols per connection).
const maxSymbols = 50

// wsSymbol validates a ticker before it is sent in a subscribe frame.
var wsSymbol = regexp.MustCompile(`^[A-Z.]{1,10}$`)

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

// Client streams live trades from Finnhub onto an outbound channel. The set of
// subscribed symbols can be extended at runtime via Subscribe.
type Client struct {
	apiKey string
	Ticks  chan db.Tick

	mu         sync.Mutex // guards symbols, subscribed, and writes to conn
	symbols    []string
	subscribed map[string]bool
	conn       *websocket.Conn // nil while disconnected
}

// NewClient builds a Finnhub WS client. The Ticks channel fans ticks out to consumers.
func NewClient(apiKey string, symbols []string) *Client {
	c := &Client{
		apiKey:     apiKey,
		Ticks:      make(chan db.Tick, 1024),
		subscribed: make(map[string]bool),
	}
	for _, s := range symbols {
		if wsSymbol.MatchString(s) && !c.subscribed[s] {
			c.subscribed[s] = true
			c.symbols = append(c.symbols, s)
		}
	}
	return c
}

// Subscribe adds a symbol to the live stream at runtime. It is safe to call
// concurrently and is idempotent. If the socket is currently connected, the
// subscribe frame is sent immediately; otherwise it is applied on next connect.
func (c *Client) Subscribe(symbol string) error {
	if !wsSymbol.MatchString(symbol) {
		return fmt.Errorf("invalid symbol")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.subscribed[symbol] {
		return nil // already streaming
	}
	if len(c.symbols) >= maxSymbols {
		return fmt.Errorf("subscription limit reached")
	}

	c.subscribed[symbol] = true
	c.symbols = append(c.symbols, symbol)

	if c.conn != nil {
		if err := c.conn.WriteJSON(subscribeMessage{Type: "subscribe", Symbol: symbol}); err != nil {
			return fmt.Errorf("subscribe %s: %w", symbol, err)
		}
	}
	log.Printf("finnhub: subscribed to %s (%d symbols)", symbol, len(c.symbols))
	return nil
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

	// Publish the live conn and (re)subscribe to all known symbols under the
	// lock so a concurrent Subscribe can't race the initial subscribe burst.
	c.mu.Lock()
	c.conn = conn
	current := append([]string(nil), c.symbols...)
	for _, sym := range current {
		if err := conn.WriteJSON(subscribeMessage{Type: "subscribe", Symbol: sym}); err != nil {
			c.conn = nil
			c.mu.Unlock()
			return fmt.Errorf("subscribe %s: %w", sym, err)
		}
	}
	c.mu.Unlock()
	log.Printf("finnhub: connected and subscribed to %v", current)

	// Clear the live conn on exit so Subscribe stops writing to a dead socket.
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()

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
