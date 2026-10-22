package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/redact"
)

func notification() alert.Notification {
	return alert.Notification{
		Kind: alert.KindFiring,
		Result: checks.Result{
			CheckID: "FW-STALE-001", Network: "ethereum", Feed: "ETH/USD", Status: checks.Critical,
			Summary:  "feed not updated for 9000s, more than 7920s",
			Details:  map[string]string{"age": "9000s", "max_age": "3960s"},
			Observed: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
		},
		Since: time.Date(2026, 10, 22, 11, 58, 0, 0, time.UTC),
		Link:  "https://etherscan.io/address/0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419",
	}
}

func single() alert.Message {
	return alert.Message{Notifications: []alert.Notification{notification()}}
}

// recorder is a scripted HTTP server.
type recorder struct {
	mu       sync.Mutex
	statuses []int
	headers  []map[string]string
	bodies   [][]byte
	paths    []string
}

func (rec *recorder) server(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.bodies = append(rec.bodies, b)
		rec.paths = append(rec.paths, r.URL.Path)
		status := http.StatusNoContent
		if len(rec.statuses) > 0 {
			status, rec.statuses = rec.statuses[0], rec.statuses[1:]
		}
		if len(rec.headers) > 0 {
			for k, v := range rec.headers[0] {
				w.Header().Set(k, v)
			}
			rec.headers = rec.headers[1:]
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

func fastPoster(p *poster, slept *[]time.Duration) {
	p.sleep = func(_ context.Context, d time.Duration) error {
		*slept = append(*slept, d)
		return nil
	}
}

func TestDiscordPayload(t *testing.T) {
	rec := &recorder{}
	s := rec.server(t)
	d := NewDiscord(s.URL+"/api/webhooks/1/secret-token-abcdefghijklmnop", nil, nil)
	if err := d.Send(context.Background(), single()); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Embeds []struct {
			Title  string `json:"title"`
			Color  int    `json:"color"`
			URL    string `json:"url"`
			Fields []struct{ Name, Value string }
		} `json:"embeds"`
		AllowedMentions struct {
			Parse []string `json:"parse"`
		} `json:"allowed_mentions"`
	}
	if err := json.Unmarshal(rec.bodies[0], &p); err != nil {
		t.Fatal(err)
	}
	e := p.Embeds[0]
	if e.Title != "[CRITICAL] FW-STALE-001 ethereum ETH/USD" || e.Color != 0xE74C3C || !strings.Contains(e.URL, "etherscan.io") {
		t.Fatalf("embed = %+v", e)
	}
	if p.AllowedMentions.Parse == nil || len(p.AllowedMentions.Parse) != 0 {
		t.Fatal("mentions must be disabled explicitly")
	}
	if bytes.Contains(rec.bodies[0], []byte("secret-token")) {
		t.Fatal("webhook token in payload")
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	rec := &recorder{statuses: []int{500, 429, 204}, headers: []map[string]string{{}, {"Retry-After": "2.5"}}}
	s := rec.server(t)
	d := NewDiscord(s.URL, nil, nil)
	var slept []time.Duration
	fastPoster(&d.p, &slept)
	if err := d.Send(context.Background(), single()); err != nil {
		t.Fatal(err)
	}
	if len(rec.bodies) != 3 || len(slept) != 2 || slept[1] != 2500*time.Millisecond {
		t.Fatalf("attempts=%d slept=%v", len(rec.bodies), slept)
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	if got := retryAfter("3600"); got != maxRetryAfter {
		t.Fatalf("retryAfter = %s", got)
	}
	if got := retryAfter("garbage"); got != 0 {
		t.Fatalf("retryAfter = %s", got)
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	rec := &recorder{statuses: []int{400, 204}}
	s := rec.server(t)
	d := NewDiscord(s.URL, nil, nil)
	var slept []time.Duration
	fastPoster(&d.p, &slept)
	if err := d.Send(context.Background(), single()); err == nil || len(rec.bodies) != 1 {
		t.Fatalf("err=%v attempts=%d", err, len(rec.bodies))
	}
}

func TestErrorsDoNotLeakSecrets(t *testing.T) {
	r := &redact.Redactor{}
	webhook := "http://127.0.0.1:1/api/webhooks/123456789/AbCdEfGhIjKlMnOpQrStUvWxYz"
	token := "123456789:AAH-telegram-token-value-xyz"
	r.Add(webhook)
	r.Add(token)
	d := NewDiscord(webhook, nil, r)
	tg := NewTelegram("http://127.0.0.1:1", token, "42", nil, r)
	var slept []time.Duration
	fastPoster(&d.p, &slept)
	fastPoster(&tg.p, &slept)
	for _, n := range []alert.Notifier{d, tg} {
		err := n.Send(context.Background(), single())
		if err == nil {
			t.Fatalf("%s: expected connection error", n.Name())
		}
		for _, secret := range []string{"AbCdEfGhIjKlMnOpQrStUvWxYz", token} {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("%s error leaks a secret: %v", n.Name(), err)
			}
		}
	}
}

func TestTelegram(t *testing.T) {
	rec := &recorder{statuses: []int{200}}
	s := rec.server(t)
	tg := NewTelegram(s.URL, "123:tok", "-1001", nil, nil)
	if err := tg.Send(context.Background(), single()); err != nil {
		t.Fatal(err)
	}
	if rec.paths[0] != "/bot123:tok/sendMessage" {
		t.Fatalf("path = %s", rec.paths[0])
	}
	var p map[string]any
	_ = json.Unmarshal(rec.bodies[0], &p)
	if p["chat_id"] != "-1001" || !strings.Contains(p["text"].(string), "FW-STALE-001") || p["parse_mode"] != nil {
		t.Fatalf("payload = %v", p)
	}
}

func TestStormAndResolvedFormatting(t *testing.T) {
	var buf bytes.Buffer
	out := NewStdout(&buf)
	warn := notification()
	warn.Result.Status = checks.Warning
	warn.Result.CheckID = "FW-REF-001"
	storm := alert.Message{Storm: true, Notifications: []alert.Notification{warn, notification()}}
	if err := out.Send(context.Background(), storm); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if lines[0] != "ALERT STORM: 2 alerts" || !strings.Contains(lines[1], "CRITICAL") {
		t.Fatalf("storm output:\n%s", buf.String())
	}
	res := notification()
	res.Kind = alert.KindResolved
	res.Result.Status = checks.OK
	if got := title(res); !strings.HasPrefix(got, "[RESOLVED]") {
		t.Fatalf("title = %s", got)
	}
}

func TestTruncateKeepsUTF8(t *testing.T) {
	s := strings.Repeat("é", 100) // 2 bytes each
	for n := 0; n < 20; n++ {
		got := truncate(s, n+3)
		if !utf8.ValidString(got) || len(got) > n+3 {
			t.Fatalf("truncate(%d) = %q", n+3, got)
		}
	}
}

func TestDiscordEscapesUntrustedMarkdown(t *testing.T) {
	n := notification()
	n.Result.Summary = `description() is "[claim refund](https://evil.example) @everyone"`
	n.Result.Details = map[string]string{"description": "**[x](https://evil.example)**"}
	b, err := json.Marshal(NewDiscord("http://unused", nil, nil).Payload(alert.Message{Notifications: []alert.Notification{n}}))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Embeds []struct {
			Description string
			Fields      []struct{ Value string }
		}
	}
	_ = json.Unmarshal(b, &p)
	all := p.Embeds[0].Description
	for _, f := range p.Embeds[0].Fields {
		all += f.Value
	}
	if strings.Contains(all, "](") || strings.Contains(all, "**") || strings.Contains(all, "@everyone") {
		t.Fatalf("markdown not escaped: %s", all)
	}
}
