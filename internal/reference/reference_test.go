package reference

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/clock"
)

var t0 = time.Date(2026, 10, 22, 3, 13, 31, 0, time.UTC)

// Real responses captured on 2026-10-22 (trimmed).
const (
	coinbaseETH = `{"ask":"2436.02","bid":"2436.01","volume":"97337.76","trade_id":849763941,"price":"2436.02","size":"0.27","time":"2026-10-22T03:13:31.348499182Z"}`
	krakenETH   = `{"error":[],"result":{"XETHZUSD":{"a":["2435.87000","13","13.000"],"b":["2435.86000","6","6.000"],"c":["2435.81000","0.01997717"]}}}`
	bitstampETH = `{"timestamp":"1792638813","open":"2472.54","last":"2435.64","bid":"2436.33","ask":"2436.34","market_type":"SPOT"}`
	geminiETH   = `{"bid":"2437.02000","ask":"2437.03000","last":"2437.03000","volume":{"ETH":"1144.91","USD":"2790188.94","timestamp":1792638780000}}`
)

// server serves body at any path and records the last request.
func server(t *testing.T, status int, body string) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var last atomic.Value
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last.Store(r.URL.String() + " UA=" + r.Header.Get("User-Agent"))
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "30")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s, &last
}

func sources(url string, clk clock.Clock) map[string]Source {
	c := &http.Client{Timeout: 2 * time.Second}
	return map[string]Source{
		"coinbase": NewCoinbase(c, url, clk),
		"kraken":   NewKraken(c, url, clk),
		"bitstamp": NewBitstamp(c, url, clk),
		"gemini":   NewGemini(c, url, clk),
	}
}

func rat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return r
}

func TestSourcesParseRealResponses(t *testing.T) {
	clk := clock.NewFake(t0)
	tests := []struct {
		source, body, wantMid, wantPath string
		exchangeTime                    bool
	}{
		{"coinbase", coinbaseETH, "2436.015", "/products/ETH-USD/ticker", true},
		{"kraken", krakenETH, "2435.865", "/0/public/Ticker?pair=ETHUSD", false},
		{"bitstamp", bitstampETH, "2436.335", "/api/v2/ticker/ethusd/", true},
		{"gemini", geminiETH, "2437.025", "/v1/pubticker/ethusd", false},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			s, last := server(t, http.StatusOK, tt.body)
			q, err := sources(s.URL, clk)[tt.source].Quote(context.Background(), "ETH-USD")
			if err != nil {
				t.Fatal(err)
			}
			if q.Price.Cmp(rat(tt.wantMid)) != 0 {
				t.Errorf("mid = %s, want %s", q.Price.FloatString(4), tt.wantMid)
			}
			if q.HasExchangeTime != tt.exchangeTime {
				t.Errorf("HasExchangeTime = %v", q.HasExchangeTime)
			}
			got := last.Load().(string)
			if !strings.HasPrefix(got, tt.wantPath) || !strings.Contains(got, "UA=oraclewatch") {
				t.Errorf("request = %s", got)
			}
		})
	}
}

func TestKrakenUsesXBTForBitcoin(t *testing.T) {
	s, last := server(t, http.StatusOK, strings.Replace(krakenETH, "XETHZUSD", "XXBTZUSD", 1))
	if _, err := NewKraken(nil, s.URL, nil).Quote(context.Background(), "BTC-USD"); err != nil {
		t.Fatal(err)
	}
	if got := last.Load().(string); !strings.Contains(got, "pair=XBTUSD") {
		t.Fatalf("request = %s", got)
	}
}

func TestSourceErrors(t *testing.T) {
	huge := `{"bid":"1","ask":"1","pad":"` + strings.Repeat("x", maxResponseSize) + `"}`
	tests := []struct {
		name, source string
		status       int
		body         string
		wantErr      error  // checked with errors.Is when set
		wantText     string // substring otherwise
	}{
		{"404 is unsupported pair", "gemini", 404, `'foousd' does not have available data yet`, ErrUnsupportedPair, ""},
		{"429 is rate limited", "coinbase", 429, `{}`, ErrRateLimited, ""},
		{"500", "kraken", 500, `oops`, nil, "HTTP 500"},
		{"malformed json", "coinbase", 200, `{"bid":`, nil, "decode"},
		{"html error page", "gemini", 200, `<html>bad gateway</html>`, nil, "decode"},
		{"huge body", "bitstamp", 200, huge, nil, "larger than"},
		{"zero price", "gemini", 200, `{"bid":"0","ask":"1"}`, ErrInvalidQuote, ""},
		{"negative price", "coinbase", 200, `{"bid":"-1","ask":"1"}`, ErrInvalidQuote, ""},
		{"exponent price", "coinbase", 200, `{"bid":"1e3","ask":"1e3"}`, ErrInvalidQuote, ""},
		{"empty price", "gemini", 200, `{"bid":"","ask":"1"}`, ErrInvalidQuote, ""},
		{"number instead of string", "gemini", 200, `{"bid":2437.02,"ask":2437.03}`, nil, "decode"},
		{"crossed book", "bitstamp", 200, `{"bid":"2","ask":"1"}`, ErrInvalidQuote, ""},
		// Bitstamp answers an unknown market with 200 and an array of every ticker.
		{"bitstamp array for unknown pair", "bitstamp", 200, `[{"pair":"BTC/USD","bid":"82748.10","ask":"82748.11"}]`, nil, "decode"},
		{"kraken unknown pair", "kraken", 200, `{"error":["EQuery:Unknown asset pair"]}`, ErrUnsupportedPair, ""},
		{"kraken two results", "kraken", 200, `{"error":[],"result":{"A":{"a":["1"],"b":["1"]},"B":{"a":["1"],"b":["1"]}}}`, ErrInvalidQuote, ""},
		{"kraken missing sides", "kraken", 200, `{"error":[],"result":{"A":{"a":[],"b":[]}}}`, ErrInvalidQuote, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := server(t, tt.status, tt.body)
			_, err := sources(s.URL, nil)[tt.source].Quote(context.Background(), "ETH-USD")
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err = %v, want %q", err, tt.wantText)
			}
		})
	}
}

func TestRetryAfterParsed(t *testing.T) {
	s, _ := server(t, http.StatusTooManyRequests, `{}`)
	_, err := NewCoinbase(nil, s.URL, nil).Quote(context.Background(), "ETH-USD")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 30*time.Second {
		t.Fatalf("err = %v", err)
	}
}

func TestSlowSourceTimesOut(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(s.Close)
	a := NewAggregator([]Source{NewGemini(nil, s.URL, nil)}, Options{MinSources: 1, Timeout: 100 * time.Millisecond})
	start := time.Now()
	ref := a.Get(context.Background(), "ETH-USD", []string{"gemini"})
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not applied")
	}
	if ref.Trusted || ref.Errors["gemini"] == "" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestCoinbaseHasNoUSDC(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(s.Close)
	if _, err := NewCoinbase(nil, s.URL, nil).Quote(context.Background(), "USDC-USD"); !errors.Is(err, ErrUnsupportedPair) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("no request should be made for a pair known to be unsupported")
	}
}

func TestPairValidation(t *testing.T) {
	for _, p := range []string{"ETHUSD", "ETH-USDT", "-USD", "ETH-EUR"} {
		if _, err := NewGemini(nil, "http://127.0.0.1:1", nil).Quote(context.Background(), p); !errors.Is(err, ErrUnsupportedPair) {
			t.Errorf("%s: err = %v", p, err)
		}
	}
}

// fixedSource returns a preset quote.
type fixedSource struct {
	name  string
	price string
	at    time.Time
	err   error
	calls atomic.Int32
}

func (f *fixedSource) Name() string { return f.name }
func (f *fixedSource) Quote(_ context.Context, pair string) (Quote, error) {
	f.calls.Add(1)
	if f.err != nil {
		return Quote{}, f.err
	}
	return Quote{Source: f.name, Pair: pair, Price: rat(f.price), Time: f.at, HasExchangeTime: !f.at.IsZero()}, nil
}

func agg(clk clock.Clock, srcs ...*fixedSource) (*Aggregator, []string) {
	var ss []Source
	var names []string
	for _, s := range srcs {
		ss = append(ss, s)
		names = append(names, s.name)
	}
	return NewAggregator(ss, Options{MinSources: 2, MaxSpread: big.NewRat(1, 100), MaxAge: 2 * time.Minute, Timeout: time.Second, CacheTTL: 10 * time.Second, Clock: clk}), names
}

func TestMedianOddEven(t *testing.T) {
	clk := clock.NewFake(t0)
	a, names := agg(clk, &fixedSource{name: "a", price: "100"}, &fixedSource{name: "b", price: "100.5"}, &fixedSource{name: "c", price: "100.2"})
	ref := a.Get(context.Background(), "ETH-USD", names)
	if !ref.Trusted || ref.Median.Cmp(rat("100.2")) != 0 {
		t.Fatalf("odd: %+v", ref)
	}
	if ref.Spread.Cmp(new(big.Rat).Quo(rat("0.5"), rat("100.2"))) != 0 {
		t.Fatalf("spread = %s", ref.Spread)
	}
	a, names = agg(clk, &fixedSource{name: "a", price: "100"}, &fixedSource{name: "b", price: "100.5"})
	if ref := a.Get(context.Background(), "ETH-USD", names); ref.Median.Cmp(rat("100.25")) != 0 {
		t.Fatalf("even median = %s", ref.Median.FloatString(4))
	}
}

func TestMedianResistsOneBadSource(t *testing.T) {
	// With three sources a single manipulated one cannot move the median,
	// but the spread check still marks the reference untrusted.
	clk := clock.NewFake(t0)
	a, names := agg(clk, &fixedSource{name: "a", price: "2436"}, &fixedSource{name: "b", price: "2437"}, &fixedSource{name: "evil", price: "1"})
	ref := a.Get(context.Background(), "ETH-USD", names)
	if ref.Median.Cmp(rat("2436")) != 0 {
		t.Fatalf("median = %s", ref.Median.FloatString(2))
	}
	if ref.Trusted || !strings.Contains(ref.Reason, "sources disagree") {
		t.Fatalf("a 99%% spread must make the reference untrusted: %+v", ref)
	}
}

func TestSpreadBoundaryIsInclusive(t *testing.T) {
	clk := clock.NewFake(t0)
	// spread = 1/100 exactly with median 100: 99.5 and 100.5.
	a, names := agg(clk, &fixedSource{name: "a", price: "99.5"}, &fixedSource{name: "b", price: "100.5"})
	if ref := a.Get(context.Background(), "ETH-USD", names); !ref.Trusted {
		t.Fatalf("spread equal to the limit must be trusted: %+v", ref)
	}
	a, names = agg(clk, &fixedSource{name: "a", price: "99.49999999"}, &fixedSource{name: "b", price: "100.5"})
	if ref := a.Get(context.Background(), "ETH-USD", names); ref.Trusted {
		t.Fatalf("spread just above the limit must be untrusted: %+v", ref)
	}
}

func TestTooFewSources(t *testing.T) {
	clk := clock.NewFake(t0)
	a, names := agg(clk, &fixedSource{name: "a", price: "100"}, &fixedSource{name: "b", err: errors.New("down")})
	ref := a.Get(context.Background(), "ETH-USD", names)
	if ref.Trusted || ref.Median == nil || ref.Errors["b"] == "" {
		t.Fatalf("ref = %+v", ref)
	}
	a, names = agg(clk, &fixedSource{name: "a", err: errors.New("down")}, &fixedSource{name: "b", err: errors.New("down")})
	if ref := a.Get(context.Background(), "ETH-USD", names); ref.Trusted || ref.Median != nil || ref.Reason != "no source answered" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestStaleAndFutureQuotesDropped(t *testing.T) {
	clk := clock.NewFake(t0)
	a, names := agg(clk,
		&fixedSource{name: "fresh", price: "100", at: t0.Add(-time.Minute)},
		&fixedSource{name: "stale", price: "100", at: t0.Add(-3 * time.Minute)},
		&fixedSource{name: "future", price: "100", at: t0.Add(5 * time.Minute)},
	)
	ref := a.Get(context.Background(), "ETH-USD", names)
	if len(ref.Quotes) != 1 || ref.Errors["stale"] == "" || ref.Errors["future"] == "" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestCacheAndRateLimitBackoff(t *testing.T) {
	clk := clock.NewFake(t0)
	ok := &fixedSource{name: "ok", price: "100"}
	limited := &fixedSource{name: "limited", err: &RateLimitError{RetryAfter: 30 * time.Second}}
	a, names := agg(clk, ok, limited)
	for i := 0; i < 5; i++ {
		a.Get(context.Background(), "ETH-USD", names)
	}
	if ok.calls.Load() != 1 || limited.calls.Load() != 1 {
		t.Fatalf("within TTL: calls ok=%d limited=%d, want 1 each", ok.calls.Load(), limited.calls.Load())
	}
	clk.Advance(11 * time.Second) // TTL expired, backoff still active
	a.Get(context.Background(), "ETH-USD", names)
	if ok.calls.Load() != 2 || limited.calls.Load() != 1 {
		t.Fatalf("after TTL: calls ok=%d limited=%d", ok.calls.Load(), limited.calls.Load())
	}
	clk.Advance(30 * time.Second)
	a.Get(context.Background(), "ETH-USD", names)
	if limited.calls.Load() != 2 {
		t.Fatalf("backoff should have expired: limited=%d", limited.calls.Load())
	}
}

func TestConcurrentCallersShareOneFetch(t *testing.T) {
	clk := clock.NewFake(t0)
	s := &fixedSource{name: "a", price: "100"}
	a, names := agg(clk, s)
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			a.Get(context.Background(), "ETH-USD", names)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	if s.calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", s.calls.Load())
	}
}

func TestParsePrice(t *testing.T) {
	good := map[string]string{"2436.02": "2436.02", "0.99981853": "0.99981853", "1": "1", "007.50": "7.5", ".5": "0.5", "5.": "5"}
	for in, want := range good {
		r, err := ParsePrice(in)
		if err != nil || r.Cmp(rat(want)) != 0 {
			t.Errorf("ParsePrice(%q) = %v, %v", in, r, err)
		}
	}
	for _, in := range []string{"", "0", "0.000", "-1", "+1", "1e3", "1/3", "0x10", "1.2.3", ".", "NaN", "Inf", " 1", "1,5", strings.Repeat("9", 65)} {
		if _, err := ParsePrice(in); err == nil {
			t.Errorf("ParsePrice(%q) should fail", in)
		}
	}
}

func FuzzParsePrice(f *testing.F) {
	for _, s := range []string{"2436.02", "0", "-1", "1e3", ".", "99999999999999999999.000000000001"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := ParsePrice(s)
		if err == nil && r.Sign() <= 0 {
			t.Fatalf("accepted non-positive %q", s)
		}
	})
}

func FuzzSourceResponses(f *testing.F) {
	for _, b := range []string{coinbaseETH, krakenETH, bitstampETH, geminiETH, `[]`, `{"result":{}}`, `null`} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, body string) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, body)
		}))
		defer s.Close()
		for name, src := range sources(s.URL, nil) {
			q, err := src.Quote(context.Background(), "ETH-USD")
			if err == nil && (q.Price == nil || q.Price.Sign() <= 0) {
				t.Fatalf("%s accepted %q with price %v", name, body, q.Price)
			}
		}
	})
}
