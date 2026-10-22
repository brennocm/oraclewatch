// Package reference fetches market prices from public exchange APIs and
// combines them into a reference price for the deviation check.
//
// Prices are parsed from their JSON string form straight into big.Rat; no
// value passes through float64. Responses are size-limited, time-limited and
// validated, because an exchange (or anything between it and us) is an
// untrusted input.
package reference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxResponseSize bounds every exchange response. A ticker is under 1 KiB.
const maxResponseSize = 64 << 10

// UserAgent identifies oraclewatch to exchanges.
const UserAgent = "oraclewatch (+https://github.com/brennocm/oraclewatch)"

// Errors returned by sources.
var (
	ErrUnsupportedPair = errors.New("pair not available on this source")
	ErrRateLimited     = errors.New("rate limited by source")
	ErrInvalidQuote    = errors.New("invalid quote")
)

// Quote is one source's price for one pair.
type Quote struct {
	Source string
	Pair   string   // BASE-USD
	Price  *big.Rat // mid = (bid + ask) / 2
	// Time is the exchange's timestamp when the API provides one
	// (HasExchangeTime), otherwise the time the response was received.
	Time            time.Time
	HasExchangeTime bool
}

// Source is one exchange.
type Source interface {
	Name() string
	Quote(ctx context.Context, pair string) (Quote, error)
}

// RateLimitError carries the server's Retry-After, when present.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited, retry after %s", e.RetryAfter)
	}
	return "rate limited"
}

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// baseOf returns "ETH" for "ETH-USD". Only USD quotes are supported.
func baseOf(pair string) (string, error) {
	base, quote, ok := strings.Cut(pair, "-")
	if !ok || base == "" || quote != "USD" {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedPair, pair)
	}
	return base, nil
}

// getJSON performs a GET and decodes the JSON body into out. 404 maps to
// ErrUnsupportedPair, 429 to *RateLimitError; other non-2xx are errors.
// The body is limited to maxResponseSize; a larger body is an error rather
// than a silently truncated document.
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response body
	// Drain a bounded amount so the connection can be reused.
	defer io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize)) //nolint:errcheck // best effort

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrUnsupportedPair
	case resp.StatusCode == http.StatusTooManyRequests:
		return &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseSize {
		return fmt.Errorf("response larger than %d bytes", maxResponseSize)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// parseRetryAfter accepts the delay-seconds form. The HTTP-date form is
// rarely used by APIs and falls back to the caller's default.
func parseRetryAfter(v string) time.Duration {
	s, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || s < 0 {
		return 0
	}
	return time.Duration(s) * time.Second
}

// ParsePrice parses a positive decimal string such as "2436.015" into an
// exact rational. Signs, exponents, fractions, hex and empty strings are
// rejected: exchanges send plain decimals, and anything else is suspect.
func ParsePrice(s string) (*big.Rat, error) {
	if s == "" || len(s) > 64 {
		return nil, fmt.Errorf("%w: price %q", ErrInvalidQuote, s)
	}
	dot := false
	digits := 0
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' && !dot:
			dot = true
		default:
			return nil, fmt.Errorf("%w: price %q", ErrInvalidQuote, s)
		}
	}
	if digits == 0 {
		return nil, fmt.Errorf("%w: price %q", ErrInvalidQuote, s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return nil, fmt.Errorf("%w: price %q must be > 0", ErrInvalidQuote, s)
	}
	return r, nil
}

// mid returns (bid + ask) / 2 after validating both sides of the book.
func mid(bidS, askS string) (*big.Rat, error) {
	bid, err := ParsePrice(bidS)
	if err != nil {
		return nil, fmt.Errorf("bid: %w", err)
	}
	ask, err := ParsePrice(askS)
	if err != nil {
		return nil, fmt.Errorf("ask: %w", err)
	}
	if ask.Cmp(bid) < 0 {
		return nil, fmt.Errorf("%w: crossed book (bid %s > ask %s)", ErrInvalidQuote, bidS, askS)
	}
	m := new(big.Rat).Add(bid, ask)
	return m.Quo(m, big.NewRat(2, 1)), nil
}
