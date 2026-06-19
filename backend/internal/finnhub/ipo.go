package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

// RateLimiter is the minimal limiter contract the IPO poller depends on.
// Defined here (not imported from redis) to keep this package decoupled.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

const ipoCalendarURL = "https://finnhub.io/api/v1/calendar/ipo"

const ipoPollInterval = 24 * time.Hour

// ipoResponse mirrors the Finnhub IPO calendar payload.
type ipoResponse struct {
	IPOCalendar []struct {
		Date             string  `json:"date"`
		Exchange         string  `json:"exchange"`
		Name             string  `json:"name"`
		NumberOfShares   float64 `json:"numberOfShares"`
		Price            string  `json:"price"`
		Symbol           string  `json:"symbol"`
		TotalSharesValue float64 `json:"totalSharesValue"`
	} `json:"ipoCalendar"`
}

// IPOPoller ingests the upcoming IPO calendar and scores each listing by risk.
type IPOPoller struct {
	db      *db.DB
	apiKey  string
	limiter RateLimiter
	client  *http.Client
}

// NewIPOPoller builds the poller. The limiter gates outbound Finnhub REST calls.
func NewIPOPoller(database *db.DB, apiKey string, limiter RateLimiter) *IPOPoller {
	return &IPOPoller{
		db:      database,
		apiKey:  apiKey,
		limiter: limiter,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Start runs an immediate poll, then repeats daily until ctx is done.
func (p *IPOPoller) Start(ctx context.Context) {
	p.poll(ctx)

	ticker := time.NewTicker(ipoPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *IPOPoller) poll(ctx context.Context) {
	allowed, err := p.limiter.Allow(ctx, "finnhub:rest")
	if err == nil && !allowed {
		log.Println("ipo: rate limit reached, skipping this poll")
		return
	}

	from := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	to := time.Now().AddDate(0, 0, 90).Format("2006-01-02")

	// URL host is a fixed constant; only date bounds and the key are appended.
	url := fmt.Sprintf("%s?from=%s&to=%s&token=%s", ipoCalendarURL, from, to, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("ipo: new request failed: %v", err)
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("ipo: fetch failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Never log the key or full URL; status code is enough.
		log.Printf("ipo: unexpected status %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		log.Printf("ipo: read body failed: %v", err)
		return
	}

	var data ipoResponse
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("ipo: decode failed: %v", err)
		return
	}

	n, err := p.upsert(ctx, data)
	if err != nil {
		log.Printf("ipo: upsert failed: %v", err)
		return
	}
	log.Printf("ipo: upserted %d upcoming listings", n)
}

func (p *IPOPoller) upsert(ctx context.Context, data ipoResponse) (int, error) {
	batch := &pgx.Batch{}
	queued := 0

	for _, ipo := range data.IPOCalendar {
		if ipo.Symbol == "" || ipo.Name == "" {
			continue
		}

		var expected *time.Time
		if d, err := time.Parse("2006-01-02", ipo.Date); err == nil {
			expected = &d
		}

		risk := computeRiskScore(ipo.Price, ipo.TotalSharesValue)

		batch.Queue(
			`INSERT INTO ipo_evaluations
			   (symbol, company_name, expected_date, risk_score, exchange, price_range, shares_value)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (symbol, expected_date)
			 DO UPDATE SET company_name = EXCLUDED.company_name,
			               risk_score   = EXCLUDED.risk_score,
			               exchange     = EXCLUDED.exchange,
			               price_range  = EXCLUDED.price_range,
			               shares_value = EXCLUDED.shares_value`,
			ipo.Symbol, ipo.Name, expected, risk, ipo.Exchange, ipo.Price, int64(ipo.TotalSharesValue),
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

// RiskFactor is a single contributor to an IPO's risk score, with the signed
// number of points it added and a human-readable explanation.
type RiskFactor struct {
	Label  string `json:"label"`
	Impact int    `json:"impact"`
	Detail string `json:"detail"`
}

// computeRiskScore returns just the 0-100 risk rating.
func computeRiskScore(priceRange string, totalValue float64) int {
	score, _ := ScoreWithFactors(priceRange, totalValue)
	return score
}

// ScoreWithFactors returns a 0-100 risk rating (higher = riskier) along with
// the breakdown of factors that produced it. Larger raises and tighter price
// ranges signal more institutional confidence and lower risk; small, cheap, or
// widely-ranged offerings score riskier.
func ScoreWithFactors(priceRange string, totalValue float64) (int, []RiskFactor) {
	score := 50
	factors := []RiskFactor{
		{Label: "Base score", Impact: 50, Detail: "Every listing starts at a neutral 50 / 100."},
	}

	addFactor := func(label string, impact int, detail string) {
		score += impact
		factors = append(factors, RiskFactor{Label: label, Impact: impact, Detail: detail})
	}

	switch {
	case totalValue >= 1_000_000_000:
		addFactor("Offering size", -25, "Raises over $1B signal strong institutional demand and underwriting confidence.")
	case totalValue >= 500_000_000:
		addFactor("Offering size", -15, "A $500M+ raise indicates solid institutional backing.")
	case totalValue >= 100_000_000:
		addFactor("Offering size", -5, "A $100M+ raise is a moderately sized, established offering.")
	case totalValue > 0 && totalValue < 50_000_000:
		addFactor("Offering size", 20, "Micro-cap raises under $50M are thinly capitalized and far more volatile.")
	case totalValue > 0 && totalValue < 100_000_000:
		addFactor("Offering size", 10, "Small raises under $100M carry elevated post-listing volatility.")
	default:
		addFactor("Offering size", 15, "Offering size is unknown, which adds uncertainty.")
	}

	low, high, ok := parsePriceRange(priceRange)
	if ok && low > 0 {
		width := (high - low) / low
		switch {
		case width >= 0.15:
			addFactor("Price-range width", 15, "A wide (15%+) price range suggests underwriters are unsure of fair value.")
		case width >= 0.08:
			addFactor("Price-range width", 8, "A moderately wide price range signals some pricing uncertainty.")
		}
		switch {
		case low < 5:
			addFactor("Share price", 20, "Sub-$5 pricing flags penny-stock-like risk and low liquidity.")
		case low < 10:
			addFactor("Share price", 10, "A single-digit share price is associated with higher volatility.")
		}
	} else {
		addFactor("Price guidance", 5, "No usable price range was provided, which adds uncertainty.")
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, factors
}

// parsePriceRange handles both "16.00" and "28.00-32.00" forms.
func parsePriceRange(s string) (low, high float64, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		l, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		h, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		return l, h, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, 0, false
	}
	return v, v, true
}
