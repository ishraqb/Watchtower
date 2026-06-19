package handlers

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ishraqb/Watchtower/backend/internal/db"
	"github.com/ishraqb/Watchtower/backend/internal/finnhub"
	"github.com/ishraqb/Watchtower/backend/internal/marketdata"
)

// rateLimiter is the limiter contract the API depends on (decoupled from redis).
type rateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// symbolSubscriber lets the API add tickers to the live stream at runtime.
type symbolSubscriber interface {
	Subscribe(symbol string) error
}

// API holds dependencies for the REST endpoints.
type API struct {
	db         *db.DB
	finnhubKey string
	limiter    rateLimiter
	subscriber symbolSubscriber
}

// NewAPI builds the REST API handler set.
func NewAPI(database *db.DB, finnhubKey string, limiter rateLimiter, subscriber symbolSubscriber) *API {
	return &API{db: database, finnhubKey: finnhubKey, limiter: limiter, subscriber: subscriber}
}

// symbolParam validates a user-supplied ticker before it touches a query.
var symbolParam = regexp.MustCompile(`^[A-Za-z.]{1,10}$`)

// CongressTrade is the JSON shape returned for a congressional trade.
type CongressTrade struct {
	Symbol          string    `json:"symbol"`
	Representative  string    `json:"representative"`
	TransactionDate time.Time `json:"transaction_date"`
	TransactionType string    `json:"transaction_type"`
	AmountRange     string    `json:"amount_range"`
}

// GetCongressBySymbol handles GET /api/congress/:symbol.
func (a *API) GetCongressBySymbol(c *gin.Context) {
	symbol := strings.ToUpper(c.Param("symbol"))
	if !symbolParam.MatchString(symbol) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid symbol"})
		return
	}

	// Parameterized query — user input is bound, never interpolated.
	rows, err := a.db.Pool.Query(
		c.Request.Context(),
		`SELECT symbol, representative_name, transaction_date, transaction_type, amount_range
		   FROM congressional_trades
		  WHERE symbol = $1
		  ORDER BY transaction_date DESC
		  LIMIT 200`,
		symbol,
	)
	if err != nil {
		// Generic client message; details stay server-side.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load trades"})
		return
	}
	defer rows.Close()

	trades := make([]CongressTrade, 0)
	for rows.Next() {
		var t CongressTrade
		var amount *string
		if err := rows.Scan(&t.Symbol, &t.Representative, &t.TransactionDate, &t.TransactionType, &amount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read trades"})
			return
		}
		if amount != nil {
			t.AmountRange = *amount
		}
		trades = append(trades, t)
	}

	c.JSON(http.StatusOK, gin.H{"symbol": symbol, "trades": trades})
}

// IPOEvaluation is the JSON shape returned for a scored IPO listing.
type IPOEvaluation struct {
	Symbol       string               `json:"symbol"`
	CompanyName  string               `json:"company_name"`
	ExpectedDate *time.Time           `json:"expected_date"`
	RiskScore    int                  `json:"risk_score"`
	Exchange     string               `json:"exchange"`
	PriceRange   string               `json:"price_range"`
	SharesValue  int64                `json:"shares_value"`
	Factors      []finnhub.RiskFactor `json:"factors"`
}

// GetIPOs handles GET /api/ipo.
func (a *API) GetIPOs(c *gin.Context) {
	rows, err := a.db.Pool.Query(
		c.Request.Context(),
		`SELECT symbol, company_name, expected_date, risk_score, exchange, price_range, shares_value
		   FROM ipo_evaluations
		  ORDER BY expected_date ASC NULLS LAST
		  LIMIT 200`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load IPOs"})
		return
	}
	defer rows.Close()

	ipos := make([]IPOEvaluation, 0)
	for rows.Next() {
		var e IPOEvaluation
		var exchange, priceRange *string
		var riskScore *int
		var sharesValue *int64
		if err := rows.Scan(&e.Symbol, &e.CompanyName, &e.ExpectedDate, &riskScore, &exchange, &priceRange, &sharesValue); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read IPOs"})
			return
		}
		if riskScore != nil {
			e.RiskScore = *riskScore
		}
		if exchange != nil {
			e.Exchange = *exchange
		}
		if priceRange != nil {
			e.PriceRange = *priceRange
		}
		if sharesValue != nil {
			e.SharesValue = *sharesValue
		}
		// Recompute the factor breakdown from stored inputs so the UI can
		// explain why each listing earned its score.
		_, e.Factors = finnhub.ScoreWithFactors(e.PriceRange, float64(e.SharesValue))
		ipos = append(ipos, e)
	}

	c.JSON(http.StatusOK, gin.H{"ipos": ipos})
}

// GetQuote handles GET /api/quote/:symbol, returning the last-known price and
// day stats from Finnhub. Used to populate the dashboard when markets are closed.
func (a *API) GetQuote(c *gin.Context) {
	symbol := strings.ToUpper(c.Param("symbol"))
	if !symbolParam.MatchString(symbol) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid symbol"})
		return
	}

	// Gate the outbound Finnhub call through the shared rate limiter.
	if a.limiter != nil {
		if allowed, err := a.limiter.Allow(c.Request.Context(), "finnhub:rest"); err == nil && !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded, please retry shortly"})
			return
		}
	}

	quote, err := finnhub.FetchQuote(c.Request.Context(), a.finnhubKey, symbol)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch quote"})
		return
	}
	c.JSON(http.StatusOK, quote)
}

// GetHistory handles GET /api/history/:symbol?range=1d, returning a normalized
// price series for the requested Robinhood-style range.
func (a *API) GetHistory(c *gin.Context) {
	symbol := strings.ToUpper(c.Param("symbol"))
	if !symbolParam.MatchString(symbol) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid symbol"})
		return
	}

	rangeKey := strings.ToLower(c.DefaultQuery("range", "1d"))
	if !marketdata.AllowedRange(rangeKey) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid range"})
		return
	}

	hist, err := marketdata.FetchHistory(c.Request.Context(), symbol, rangeKey)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch history"})
		return
	}
	c.JSON(http.StatusOK, hist)
}

// Watch handles POST /api/watch/:symbol, adding a ticker to the live Finnhub
// WebSocket stream so it receives real-time ticks. The symbol is validated
// before use. Intentionally unauthenticated to match the rest of this demo;
// it only subscribes to public market data and cannot read or mutate user data.
func (a *API) Watch(c *gin.Context) {
	symbol := strings.ToUpper(c.Param("symbol"))
	if !symbolParam.MatchString(symbol) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid symbol"})
		return
	}
	if a.subscriber == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live stream unavailable"})
		return
	}
	if err := a.subscriber.Subscribe(symbol); err != nil {
		// Generic client message; the detailed reason stays server-side.
		c.JSON(http.StatusConflict, gin.H{"error": "could not subscribe to symbol"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"symbol": symbol, "subscribed": true})
}
