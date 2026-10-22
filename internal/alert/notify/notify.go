// Package notify implements the alert channels: Discord webhooks, the
// Telegram Bot API and stdout.
//
// Webhook URLs and bot tokens are secrets. They are only used to build the
// request; every error is passed through the redactor before it is returned,
// because net/http errors embed the request URL.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// UserAgent identifies oraclewatch to notification APIs.
const UserAgent = "oraclewatch (+https://github.com/brennocm/oraclewatch)"

const (
	maxAttempts   = 3
	maxRetryAfter = 30 * time.Second
	maxBodyRead   = 16 << 10
)

// poster sends JSON with bounded retries. It retries network errors, 5xx and
// 429 (honouring Retry-After up to maxRetryAfter); other 4xx are permanent.
type poster struct {
	client   *http.Client
	redactor *redact.Redactor
	sleep    func(context.Context, time.Duration) error
}

func newPoster(client *http.Client, r *redact.Redactor) poster {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if r == nil {
		r = &redact.Redactor{}
	}
	return poster{client: client, redactor: r, sleep: sleepCtx}
}

func (p poster) postJSON(ctx context.Context, url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		wait, err := p.once(ctx, url, body)
		if err == nil {
			return nil
		}
		last = err
		var perm *permanentError
		if errors.As(err, &perm) || attempt == maxAttempts {
			break
		}
		if wait <= 0 {
			wait = time.Duration(attempt) * time.Second
		}
		if serr := p.sleep(ctx, wait); serr != nil {
			return p.redactor.Error(serr)
		}
	}
	return p.redactor.Error(last)
}

type permanentError struct{ status int }

func (e *permanentError) Error() string { return "HTTP " + strconv.Itoa(e.status) + " (not retried)" }

// once performs one request. It returns the server-requested wait for 429.
func (p poster) once(ctx context.Context, url string, body []byte) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close() //nolint:errcheck // response body, read-only
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyRead))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		return 0, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return retryAfter(resp.Header.Get("Retry-After")), errors.New("HTTP 429")
	case resp.StatusCode >= 500:
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return 0, &permanentError{status: resp.StatusCode}
}

// retryAfter parses delay-seconds (decimals allowed, as Discord sends) and
// caps it.
func retryAfter(v string) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f < 0 {
		return 0
	}
	d := time.Duration(f * float64(time.Second))
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// --- shared formatting ---

func statusLabel(n alert.Notification) string {
	switch n.Kind {
	case alert.KindResolved:
		return "RESOLVED"
	case alert.KindEvent:
		return "INFO"
	}
	label := strings.ToUpper(n.Result.Status.String())
	if n.Previous != checks.OK && n.Result.Status > n.Previous {
		label += " (escalated from " + n.Previous.String() + ")"
	} else if n.Reminder {
		label += " (still firing)"
	}
	return label
}

func subject(r checks.Result) string {
	parts := []string{}
	if r.Network != "" {
		parts = append(parts, r.Network)
	}
	if r.Feed != "" {
		parts = append(parts, r.Feed)
	}
	return strings.Join(parts, " ")
}

// title is one line: "[CRITICAL] FW-STALE-001 ethereum ETH/USD".
func title(n alert.Notification) string {
	return "[" + statusLabel(n) + "] " + n.Result.CheckID + " " + subject(n.Result)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stormLines summarises a storm, one line per alert, worst first.
func stormLines(m alert.Message) []string {
	ns := append([]alert.Notification(nil), m.Notifications...)
	sort.SliceStable(ns, func(i, j int) bool { return ns[i].Result.Status > ns[j].Result.Status })
	lines := make([]string, len(ns))
	for i, n := range ns {
		lines[i] = title(n) + ": " + n.Result.Summary
	}
	return lines
}

// truncate limits s to n bytes without splitting a UTF-8 sequence. Byte
// length is a safe upper bound for limits expressed in characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const ellipsis = "…"
	cut := max(n-len(ellipsis), 0)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + ellipsis
}
