package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

const quoteURL = "https://finnhub.io/api/v1/quote"

// quoteSymbol validates a symbol before it is placed in the outbound URL (SSRF-safe).
var quoteSymbol = regexp.MustCompile(`^[A-Z.]{1,10}$`)

var quoteClient = &http.Client{Timeout: 15 * time.Second}

// Quote is the last-known price snapshot for a symbol.
type Quote struct {
	Symbol        string  `json:"symbol"`
	Current       float64 `json:"current"`
	Change        float64 `json:"change"`
	PercentChange float64 `json:"percent_change"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Open          float64 `json:"open"`
	PreviousClose float64 `json:"previous_close"`
}

// finnhubQuote mirrors the raw Finnhub /quote response.
type finnhubQuote struct {
	C  float64 `json:"c"`  // current price
	D  float64 `json:"d"`  // change
	DP float64 `json:"dp"` // percent change
	H  float64 `json:"h"`  // day high
	L  float64 `json:"l"`  // day low
	O  float64 `json:"o"`  // open
	PC float64 `json:"pc"` // previous close
}

// FetchQuote returns the latest quote for a symbol. The host is a fixed
// constant; only the validated symbol and the API key are appended.
func FetchQuote(ctx context.Context, apiKey, symbol string) (Quote, error) {
	if !quoteSymbol.MatchString(symbol) {
		return Quote{}, fmt.Errorf("invalid symbol")
	}

	url := fmt.Sprintf("%s?symbol=%s&token=%s", quoteURL, symbol, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Quote{}, err
	}

	resp, err := quoteClient.Do(req)
	if err != nil {
		return Quote{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Status only — never echo the key-bearing URL.
		return Quote{}, fmt.Errorf("quote request failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Quote{}, err
	}

	var raw finnhubQuote
	if err := json.Unmarshal(body, &raw); err != nil {
		return Quote{}, err
	}

	return Quote{
		Symbol:        symbol,
		Current:       raw.C,
		Change:        raw.D,
		PercentChange: raw.DP,
		High:          raw.H,
		Low:           raw.L,
		Open:          raw.O,
		PreviousClose: raw.PC,
	}, nil
}
