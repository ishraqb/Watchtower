package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// baseURL is a fixed, trusted host. Only a validated symbol and allow-listed
// query params are ever appended, preventing SSRF via user input.
const baseURL = "https://query1.finance.yahoo.com/v8/finance/chart/"

// historySymbol validates a ticker before it is placed in the outbound URL.
var historySymbol = regexp.MustCompile(`^[A-Z.]{1,10}$`)

var httpClient = &http.Client{Timeout: 20 * time.Second}

// Point is a single (time, price) sample in a history series.
type Point struct {
	Time  int64   `json:"time"`  // unix seconds
	Value float64 `json:"value"` // close price
}

// History is the normalized response returned to the frontend.
type History struct {
	Symbol        string  `json:"symbol"`
	Range         string  `json:"range"`
	PreviousClose float64 `json:"previous_close"`
	Points        []Point `json:"points"`
}

// rangeSpec maps a user-facing range to fixed Yahoo range/interval values and
// an optional trailing-window trim (used for the intraday "1h" view).
type rangeSpec struct {
	yahooRange string
	interval   string
	trimWindow time.Duration // 0 means keep all points
}

// allowedRanges is a strict allow-list. The user-supplied range key can only
// select one of these pre-defined, validated specs.
var allowedRanges = map[string]rangeSpec{
	"1h":  {yahooRange: "1d", interval: "1m", trimWindow: time.Hour},
	"1d":  {yahooRange: "1d", interval: "5m"},
	"1w":  {yahooRange: "5d", interval: "30m"},
	"ytd": {yahooRange: "ytd", interval: "1d"},
	"1y":  {yahooRange: "1y", interval: "1d"},
	"5y":  {yahooRange: "5y", interval: "1wk"},
	"max": {yahooRange: "max", interval: "1mo"},
}

// AllowedRange reports whether a range key is valid.
func AllowedRange(key string) bool {
	_, ok := allowedRanges[key]
	return ok
}

// yahooChart mirrors the subset of the Yahoo chart payload we consume.
type yahooChart struct {
	Chart struct {
		Result []struct {
			Meta struct {
				ChartPreviousClose float64 `json:"chartPreviousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

// FetchHistory returns a normalized price series for a symbol and range key.
func FetchHistory(ctx context.Context, symbol, rangeKey string) (History, error) {
	if !historySymbol.MatchString(symbol) {
		return History{}, fmt.Errorf("invalid symbol")
	}
	spec, ok := allowedRanges[rangeKey]
	if !ok {
		return History{}, fmt.Errorf("invalid range")
	}

	// Host is constant; symbol and params are validated/allow-listed above.
	url := fmt.Sprintf("%s%s?range=%s&interval=%s", baseURL, symbol, spec.yahooRange, spec.interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return History{}, err
	}
	// Yahoo rejects requests without a browser-like User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Watchtower/1.0)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return History{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return History{}, fmt.Errorf("history request failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return History{}, err
	}

	var parsed yahooChart
	if err := json.Unmarshal(body, &parsed); err != nil {
		return History{}, err
	}
	if len(parsed.Chart.Result) == 0 {
		return History{}, fmt.Errorf("no data for symbol")
	}

	res := parsed.Chart.Result[0]
	out := History{
		Symbol:        symbol,
		Range:         rangeKey,
		PreviousClose: res.Meta.ChartPreviousClose,
	}

	var closes []*float64
	if len(res.Indicators.Quote) > 0 {
		closes = res.Indicators.Quote[0].Close
	}

	for i, ts := range res.Timestamp {
		if i >= len(closes) || closes[i] == nil {
			continue // gaps (e.g. pre/post-market or halts) are skipped
		}
		out.Points = append(out.Points, Point{Time: ts, Value: *closes[i]})
	}

	// Trim to a trailing window for the intraday "1h" view.
	if spec.trimWindow > 0 && len(out.Points) > 0 {
		cutoff := out.Points[len(out.Points)-1].Time - int64(spec.trimWindow.Seconds())
		trimmed := out.Points[:0:0]
		for _, p := range out.Points {
			if p.Time >= cutoff {
				trimmed = append(trimmed, p)
			}
		}
		out.Points = trimmed
	}

	return out, nil
}
