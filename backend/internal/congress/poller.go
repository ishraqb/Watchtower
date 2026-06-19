// Package congress pulls congressional stock disclosures on a schedule and
// upserts them so the API can serve "who in Congress traded this ticker".
package congress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

// dataBaseURL is the trusted, hardcoded source for congressional disclosures.
// SSRF note: this host is a fixed constant and is never built from user input.
const dataBaseURL = "https://raw.githubusercontent.com/kadoa-org/congress-trading-monitor/main/public/data/ticker/"

// WatchlistSymbols are the tickers we ingest congressional trades for.
// Kept as a fixed allowlist so the outbound URL is never user-controlled.
var WatchlistSymbols = []string{"AAPL", "TSLA", "RIVN", "NVDA", "MSFT", "AMZN", "GOOGL", "META"}

const pollInterval = 6 * time.Hour

// symbolPattern guards the URL path segment even though symbols come from a
// fixed allowlist — defense in depth against path traversal / SSRF.
var symbolPattern = regexp.MustCompile(`^[A-Z.]{1,10}$`)

// tickerFile mirrors the per-ticker JSON document shape.
type tickerFile struct {
	Ticker string `json:"ticker"`
	Trades []struct {
		TransactionDate string `json:"transaction_date"`
		TransactionType string `json:"transaction_type"`
		AmountLabel     string `json:"amount_range_label"`
		FilerName       string `json:"filer_name"`
		Ticker          string `json:"ticker"`
	} `json:"trades"`
}

// Poller periodically ingests congressional trades into the database.
type Poller struct {
	db     *db.DB
	client *http.Client
}

// NewPoller builds a poller with a bounded HTTP timeout.
func NewPoller(database *db.DB) *Poller {
	return &Poller{
		db:     database,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Start runs an immediate poll, then repeats every pollInterval until ctx is done.
func (p *Poller) Start(ctx context.Context) {
	p.pollAll(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	total := 0
	for _, sym := range WatchlistSymbols {
		n, err := p.pollSymbol(ctx, sym)
		if err != nil {
			log.Printf("congress: poll %s failed: %v", sym, err)
			continue
		}
		total += n
	}
	log.Printf("congress: upserted %d trades across %d symbols", total, len(WatchlistSymbols))
}

func (p *Poller) pollSymbol(ctx context.Context, symbol string) (int, error) {
	if !symbolPattern.MatchString(symbol) {
		return 0, fmt.Errorf("invalid symbol %q", symbol)
	}

	url := dataBaseURL + symbol + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, nil // no disclosures for this ticker yet
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // cap at 16 MB
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	var doc tickerFile
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, fmt.Errorf("decode json: %w", err)
	}

	return p.upsert(ctx, symbol, doc)
}

func (p *Poller) upsert(ctx context.Context, symbol string, doc tickerFile) (int, error) {
	batch := &pgx.Batch{}
	queued := 0

	for _, t := range doc.Trades {
		txType := normalizeType(t.TransactionType)
		if txType == "" {
			continue // skip exchanges, gifts, and other non-buy/sell rows
		}

		date, err := time.Parse("2006-01-02", t.TransactionDate)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(t.FilerName)
		if name == "" {
			continue
		}

		// Parameterized upsert — never string-interpolated — per SQL safety rules.
		batch.Queue(
			`INSERT INTO congressional_trades
			   (symbol, representative_name, transaction_date, transaction_type, amount_range)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (symbol, representative_name, transaction_date)
			 DO UPDATE SET transaction_type = EXCLUDED.transaction_type,
			               amount_range = EXCLUDED.amount_range`,
			symbol, name, date, txType, t.AmountLabel,
		)
		queued++
	}

	if queued == 0 {
		return 0, nil
	}

	br := p.db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			return 0, fmt.Errorf("batch exec: %w", err)
		}
	}
	return queued, nil
}

// normalizeType maps disclosure transaction types to the BUY/SELL enum.
func normalizeType(raw string) string {
	r := strings.ToLower(raw)
	switch {
	case strings.Contains(r, "purchase"):
		return "BUY"
	case strings.Contains(r, "sale"), strings.Contains(r, "sell"):
		return "SELL"
	default:
		return ""
	}
}
