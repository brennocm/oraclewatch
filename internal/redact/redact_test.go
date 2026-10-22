package redact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	rpcURL     = "https://eth-mainnet.example.com/v2/sk_live_9f8e7d6c5b4a3210"
	rpcKey     = "sk_live_9f8e7d6c5b4a3210"
	webhookURL = "https://discord.com/api/webhooks/123456789012/AbCdEfGhIjKlMnOpQrStUvWxYz"
	botToken   = "123456789:AAH-telegram-token-value"
)

func newRedactor() *Redactor {
	r := &Redactor{}
	for _, s := range []string{rpcURL, webhookURL, botToken} {
		r.Add(s)
	}
	return r
}

func TestStringRedactsSecretsAndTheirParts(t *testing.T) {
	r := newRedactor()
	tests := []struct {
		name, in string
	}{
		{"full rpc url", "dial " + rpcURL + " failed"},
		{"api key alone", "key=" + rpcKey},
		{"webhook", "post " + webhookURL},
		{"webhook path only", "path api/webhooks/123456789012/AbCdEfGhIjKlMnOpQrStUvWxYz"},
		{"bot token in telegram url", "https://api.telegram.org/bot" + botToken + "/sendMessage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.String(tt.in)
			for _, secret := range []string{rpcKey, "AbCdEfGhIjKlMnOpQrStUvWxYz", botToken} {
				if strings.Contains(got, secret) {
					t.Fatalf("String(%q) = %q still contains a secret", tt.in, got)
				}
			}
			if !strings.Contains(got, Placeholder) {
				t.Fatalf("String(%q) = %q has no placeholder", tt.in, got)
			}
		})
	}
}

func TestShortValuesAreIgnored(t *testing.T) {
	r := &Redactor{}
	r.Add("")
	r.Add("12345")
	if got := r.String("chat 12345"); got != "chat 12345" {
		t.Fatalf("short value was redacted: %q", got)
	}
}

func TestErrorRewritesURLErrorEvenWhenUnregistered(t *testing.T) {
	r := &Redactor{} // nothing registered
	unregistered := "https://rpc.example.com/secret-key-not-registered"
	err := fmt.Errorf("calling endpoint primary: %w", &url.Error{Op: "Post", URL: unregistered, Err: context.DeadlineExceeded})

	got := r.Error(err)
	if strings.Contains(got.Error(), "secret-key-not-registered") {
		t.Fatalf("url leaked: %q", got)
	}
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(DeadlineExceeded) lost through redaction: %v", got)
	}
}

func TestErrorRealHTTPClientFailure(t *testing.T) {
	r := newRedactor()
	// A real failing request: the stdlib error includes the full URL.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:1/v2/"+rpcKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Skip("port 1 unexpectedly accepted a connection")
	}
	if !strings.Contains(err.Error(), rpcKey) {
		t.Fatalf("precondition: stdlib error should contain the URL, got %q", err)
	}
	if got := r.Error(err).Error(); strings.Contains(got, rpcKey) {
		t.Fatalf("redacted error still contains the key: %q", got)
	}
}

func TestErrorKeepsCleanErrorsIntact(t *testing.T) {
	r := newRedactor()
	sentinel := errors.New("no healthy endpoint")
	wrapped := fmt.Errorf("network ethereum: %w", sentinel)
	if got := r.Error(wrapped); got != wrapped { //nolint:errorlint // identity (not equivalence) is the property under test
		t.Fatalf("clean error was rewritten: %v", got)
	}
	if r.Error(nil) != nil {
		t.Fatal("nil error must stay nil")
	}
}

func TestErrorRewritesErrorsContainingSecrets(t *testing.T) {
	r := newRedactor()
	err := fmt.Errorf("bad config value %q", webhookURL)
	got := r.Error(err)
	if strings.Contains(got.Error(), "AbCdEf") {
		t.Fatalf("secret leaked: %q", got)
	}
	if errors.Unwrap(got) != nil {
		t.Fatal("rewritten error must not expose the original through Unwrap")
	}
}

type leakyStringer struct{}

func (leakyStringer) String() string { return "endpoint " + rpcURL }

func TestHandlerRedactsMessageAndAttributes(t *testing.T) {
	r := newRedactor()
	var buf bytes.Buffer
	logger := slog.New(NewHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}), r))

	logger = logger.With("preset", rpcURL).WithGroup("g")
	logger.Debug("dialing "+rpcURL,
		"url", rpcURL,
		"err", &url.Error{Op: "Post", URL: rpcURL, Err: errors.New("EOF")},
		"stringer", leakyStringer{},
		slog.Group("nested", "token", botToken),
		"count", 3,
	)
	out := buf.String()
	for _, secret := range []string{rpcKey, botToken} {
		if strings.Contains(out, secret) {
			t.Fatalf("log output contains secret %q:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, `"count":3`) {
		t.Fatalf("non-string attribute lost: %s", out)
	}
}

func FuzzString(f *testing.F) {
	f.Add("prefix " + rpcURL + " suffix")
	f.Add(rpcKey + rpcKey)
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		r := newRedactor()
		got := r.String(s)
		if strings.Contains(got, rpcKey) || strings.Contains(got, botToken) {
			t.Fatalf("String(%q) = %q contains a secret", s, got)
		}
	})
}
