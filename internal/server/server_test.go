package server

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/engine"
	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/metrics"
)

type fakeSource struct{ ready bool }

func (f fakeSource) Ready() bool { return f.ready }
func (fakeSource) Health() []engine.NetworkHealth {
	return []engine.NetworkHealth{{Network: "ethereum", Healthy: true, Block: 100, Endpoints: 2}}
}
func (fakeSource) Results() []checks.Result {
	return []checks.Result{{CheckID: "FW-STALE-001", Network: "ethereum", Feed: "ETH/USD", Status: checks.Warning, Summary: "old", Observed: time.Unix(0, 0)}}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestEndpoints(t *testing.T) {
	m := metrics.New()
	m.ObserveFeed("ethereum", "ETH/USD", &feeds.Reading{Price: big.NewRat(244363658508, 100000000),
		Round: feeds.Round{RoundID: big.NewInt(5), UpdatedAt: big.NewInt(1792635539)}}, 30*time.Second)
	m.ObserveRPC("ethereum", "primary", "eth_call", "ok", 50*time.Millisecond)
	s := New(Options{Listen: "127.0.0.1:0", Registry: m.Registry, Source: fakeSource{ready: true}, Version: "test"})
	h := s.Handler()

	if rec := get(t, h, "/healthz"); rec.Code != 200 || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("healthz: %d %v", rec.Code, rec.Header())
	}
	if rec := get(t, h, "/readyz"); rec.Code != 200 {
		t.Fatalf("readyz: %d", rec.Code)
	}
	rec := get(t, h, "/status")
	var st struct {
		Results []struct{ Check, Status string } `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || len(st.Results) != 1 || st.Results[0].Status != "warning" {
		t.Fatalf("status: %s", rec.Body.String())
	}
	body := get(t, h, "/metrics").Body.String()
	for _, want := range []string{`oraclewatch_feed_answer{feed="ETH/USD",network="ethereum"} 2443.63658508`, `oraclewatch_rpc_requests_total{endpoint="primary"`} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/status", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /status = %d", post.Code)
	}
}

func TestNotReady(t *testing.T) {
	s := New(Options{Listen: "127.0.0.1:0", Registry: metrics.New().Registry, Source: fakeSource{ready: false}})
	if rec := get(t, s.Handler(), "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz: %d", rec.Code)
	}
}
