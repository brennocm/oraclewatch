package reference

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/brennocm/oraclewatch/internal/clock"
)

// Default API base URLs. Tests replace them with httptest servers.
const (
	CoinbaseURL = "https://api.exchange.coinbase.com"
	KrakenURL   = "https://api.kraken.com"
	BitstampURL = "https://www.bitstamp.net"
	GeminiURL   = "https://api.gemini.com"
)

type base struct {
	name   string
	url    string
	client *http.Client
	clock  clock.Clock
}

func newBase(name, url string, client *http.Client, clk clock.Clock) base {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if clk == nil {
		clk = clock.Real{}
	}
	return base{name: name, url: strings.TrimRight(url, "/"), client: client, clock: clk}
}

func (b base) Name() string { return b.name }

// Coinbase is the Coinbase Exchange public ticker.
// GET /products/{BASE}-USD/ticker → {"bid","ask","price","time",...}
type Coinbase struct{ base }

// NewCoinbase returns the Coinbase source. An empty url selects CoinbaseURL.
func NewCoinbase(client *http.Client, url string, clk clock.Clock) *Coinbase {
	if url == "" {
		url = CoinbaseURL
	}
	return &Coinbase{newBase("coinbase", url, client, clk)}
}

// Quote implements Source.
func (s *Coinbase) Quote(ctx context.Context, pair string) (Quote, error) {
	baseAsset, err := baseOf(pair)
	if err != nil {
		return Quote{}, err
	}
	// Coinbase treats USDC as USD and lists no USDC-USD market (verified
	// 2026-10-22: 404), so it cannot be a USDC reference.
	if baseAsset == "USDC" {
		return Quote{}, ErrUnsupportedPair
	}
	var t struct {
		Bid  string `json:"bid"`
		Ask  string `json:"ask"`
		Time string `json:"time"`
	}
	if err := getJSON(ctx, s.client, s.url+"/products/"+baseAsset+"-USD/ticker", &t); err != nil {
		return Quote{}, err
	}
	price, err := mid(t.Bid, t.Ask)
	if err != nil {
		return Quote{}, err
	}
	// "time" is the last trade. An old last trade on a liquid pair means the
	// data is not current, so it is used as the quote time.
	q := Quote{Source: s.name, Pair: pair, Price: price, Time: s.clock.Now()}
	if ts, err := time.Parse(time.RFC3339Nano, t.Time); err == nil {
		q.Time, q.HasExchangeTime = ts, true
	}
	return q, nil
}

// Kraken is the Kraken public Ticker endpoint.
// GET /0/public/Ticker?pair=ETHUSD → {"error":[],"result":{"XETHZUSD":{"a":[ask,..],"b":[bid,..],...}}}
type Kraken struct{ base }

// NewKraken returns the Kraken source. An empty url selects KrakenURL.
func NewKraken(client *http.Client, url string, clk clock.Clock) *Kraken {
	if url == "" {
		url = KrakenURL
	}
	return &Kraken{newBase("kraken", url, client, clk)}
}

// Quote implements Source.
func (s *Kraken) Quote(ctx context.Context, pair string) (Quote, error) {
	baseAsset, err := baseOf(pair)
	if err != nil {
		return Quote{}, err
	}
	if baseAsset == "BTC" {
		baseAsset = "XBT" // Kraken's code for bitcoin
	}
	var r struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			A []string `json:"a"`
			B []string `json:"b"`
		} `json:"result"`
	}
	if err := getJSON(ctx, s.client, s.url+"/0/public/Ticker?pair="+baseAsset+"USD", &r); err != nil {
		return Quote{}, err
	}
	if len(r.Error) > 0 {
		if strings.Contains(strings.Join(r.Error, ";"), "Unknown asset pair") {
			return Quote{}, ErrUnsupportedPair
		}
		return Quote{}, fmt.Errorf("kraken error: %s", strings.Join(r.Error, "; "))
	}
	// One pair was requested, so exactly one result is expected. Its key is
	// Kraken's canonical name (XETHZUSD), which is why it is not looked up.
	if len(r.Result) != 1 {
		return Quote{}, fmt.Errorf("%w: %d results for one pair", ErrInvalidQuote, len(r.Result))
	}
	var ask, bid []string
	for _, t := range r.Result {
		ask, bid = t.A, t.B
	}
	if len(ask) == 0 || len(bid) == 0 {
		return Quote{}, fmt.Errorf("%w: missing bid/ask", ErrInvalidQuote)
	}
	price, err := mid(bid[0], ask[0])
	if err != nil {
		return Quote{}, err
	}
	return Quote{Source: s.name, Pair: pair, Price: price, Time: s.clock.Now()}, nil
}

// Bitstamp is the Bitstamp v2 ticker.
// GET /api/v2/ticker/ethusd/ → {"bid","ask","timestamp",...}
//
// An unknown market returns HTTP 200 with an array of every ticker (observed
// 2026-10-22), so the response is decoded strictly as one object.
type Bitstamp struct{ base }

// NewBitstamp returns the Bitstamp source. An empty url selects BitstampURL.
func NewBitstamp(client *http.Client, url string, clk clock.Clock) *Bitstamp {
	if url == "" {
		url = BitstampURL
	}
	return &Bitstamp{newBase("bitstamp", url, client, clk)}
}

// Quote implements Source.
func (s *Bitstamp) Quote(ctx context.Context, pair string) (Quote, error) {
	baseAsset, err := baseOf(pair)
	if err != nil {
		return Quote{}, err
	}
	var t struct {
		Bid       string `json:"bid"`
		Ask       string `json:"ask"`
		Timestamp string `json:"timestamp"`
	}
	if err := getJSON(ctx, s.client, s.url+"/api/v2/ticker/"+strings.ToLower(baseAsset)+"usd/", &t); err != nil {
		return Quote{}, err
	}
	price, err := mid(t.Bid, t.Ask)
	if err != nil {
		return Quote{}, err
	}
	q := Quote{Source: s.name, Pair: pair, Price: price, Time: s.clock.Now()}
	if sec, err := strconv.ParseInt(t.Timestamp, 10, 64); err == nil && sec > 0 {
		q.Time, q.HasExchangeTime = time.Unix(sec, 0), true
	}
	return q, nil
}

// Gemini is the Gemini v1 public ticker.
// GET /v1/pubticker/ethusd → {"bid","ask","last","volume":{...}}
// volume.timestamp is a coarse bucket, not a quote time, so it is not used.
type Gemini struct{ base }

// NewGemini returns the Gemini source. An empty url selects GeminiURL.
func NewGemini(client *http.Client, url string, clk clock.Clock) *Gemini {
	if url == "" {
		url = GeminiURL
	}
	return &Gemini{newBase("gemini", url, client, clk)}
}

// Quote implements Source.
func (s *Gemini) Quote(ctx context.Context, pair string) (Quote, error) {
	baseAsset, err := baseOf(pair)
	if err != nil {
		return Quote{}, err
	}
	var t struct {
		Bid string `json:"bid"`
		Ask string `json:"ask"`
	}
	if err := getJSON(ctx, s.client, s.url+"/v1/pubticker/"+strings.ToLower(baseAsset)+"usd", &t); err != nil {
		return Quote{}, err
	}
	price, err := mid(t.Bid, t.Ask)
	if err != nil {
		return Quote{}, err
	}
	return Quote{Source: s.name, Pair: pair, Price: price, Time: s.clock.Now()}, nil
}

// NewSource returns the named source with default URLs.
func NewSource(name string, client *http.Client, clk clock.Clock) (Source, error) {
	switch name {
	case "coinbase":
		return NewCoinbase(client, "", clk), nil
	case "kraken":
		return NewKraken(client, "", clk), nil
	case "bitstamp":
		return NewBitstamp(client, "", clk), nil
	case "gemini":
		return NewGemini(client, "", clk), nil
	}
	return nil, fmt.Errorf("unknown reference source %q", name)
}
