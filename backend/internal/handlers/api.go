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
)

// rateLimiter is the limiter contract the API depends on (decoupled from redis).
type rateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// API holds dependencies for the REST endpoints.
type API struct {
	db         *db.DB
	finnhubKey string
	limiter    rateLimiter
}

// NewAPI builds the REST API handler set.
func NewAPI(database *db.DB, finnhubKey string, limiter rateLimiter) *API {
	return &API{db: database, finnhubKey: finnhubKey, limiter: limiter}
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
	Symbol       string     `json:"symbol"`
	CompanyName  string     `json:"company_name"`
	ExpectedDate *time.Time `json:"expected_date"`
	RiskScore    int        `json:"risk_score"`
	Exchange     string     `json:"exchange"`
	PriceRange   string     `json:"price_range"`
	SharesValue  int64      `json:"shares_value"`
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
