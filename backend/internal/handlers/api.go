package handlers

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

// API holds dependencies for the REST endpoints.
type API struct {
	db *db.DB
}

// NewAPI builds the REST API handler set.
func NewAPI(database *db.DB) *API {
	return &API{db: database}
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
