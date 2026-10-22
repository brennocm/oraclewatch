package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/server"
)

// TestSecretsNeverLeak injects known secrets into every place a secret can
// live and checks that none of them appears in logs (debug level), stdout
// notifications, Prometheus metrics, /status, or the bodies sent to the
// notification services, including when those requests fail and the stdlib
// error embeds the request URL.
func TestSecretsNeverLeak(t *testing.T) {
	secrets := map[string]string{
		"rpc key (working endpoint)": "sk_live_working_0123456789abcdef",
		"rpc key (dead endpoint)":    "sk_live_dead_fedcba9876543210",
		"discord token":              "DiscordTokenAbCdEfGhIjKlMnOpQrSt",
		"telegram token":             "987654321:AAH-telegram-secret-token-value",
		"deadman token":              "deadman-uuid-11111111-2222-3333",
	}

	chain := httptest.NewServer(e2eChain(1, 3*time.Hour, false).Handler()) // stale feed: produces alerts
	t.Cleanup(chain.Close)

	var mu sync.Mutex
	var notifyBodies bytes.Buffer
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		notifyBodies.Write(b)
		mu.Unlock()
		w.WriteHeader(http.StatusBadGateway) // Discord/Telegram/deadman are down
	}))
	t.Cleanup(down.Close)

	t.Setenv("RPC_GOOD", chain.URL+"/v2/"+secrets["rpc key (working endpoint)"])
	t.Setenv("RPC_DEAD", "http://127.0.0.1:1/v2/"+secrets["rpc key (dead endpoint)"])
	t.Setenv("DISCORD", down.URL+"/api/webhooks/1/"+secrets["discord token"])
	t.Setenv("TG_TOKEN", secrets["telegram token"])
	t.Setenv("DEADMAN", down.URL+"/ping/"+secrets["deadman token"])

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := `
networks:
  - name: ethereum
    chain_id: 1
    rpc_endpoints: [{name: good, url: "${RPC_GOOD}"}, {name: dead, url: "${RPC_DEAD}"}]
    poll_interval: 15s
    call_timeout: 2s
    explorer_url: https://etherscan.io
feeds:
  - {name: ETH/USD, network: ethereum, proxy: "0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419", expected_decimals: 8, heartbeat: 1h, deviation_threshold: 0.5%}
alerting:
  for: 0s
  discord: {webhook_url: "${DISCORD}"}
  telegram: {bot_token: "${TG_TOKEN}", chat_id: "42"}
self_health: {deadman_url: "${DEADMAN}"}
log: {level: debug, format: json}
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	var logs, stdout bytes.Buffer
	ctx := context.Background()
	a, err := loadApp(ctx, cfgPath, &logs, true)
	if err != nil {
		t.Fatal(err)
	}
	manager := alert.NewManager(alert.Options{For: 0, Cooldown: time.Hour, SendResolved: true, Link: a.link})
	disp := alert.NewDispatcher(a.notifiers(&stdout), alert.DispatcherOptions{GroupWindow: 10 * time.Millisecond, StormThreshold: 50, Logger: a.logger, Observer: a.metrics})
	eng, err := a.engine(manager, disp, a.deadman())
	if err != nil {
		t.Fatal(err)
	}

	dctx, dcancel := context.WithCancel(ctx)
	go disp.Run(dctx)
	results := eng.RunOnce(ctx)
	disp.Enqueue(ctx, manager.Process(results, time.Now()))
	if err := a.deadman()(ctx); err != nil {
		a.logger.Error("deadman", "err", err)
	}
	time.Sleep(200 * time.Millisecond)
	dcancel()
	<-disp.Done()

	srv := server.New(server.Options{Listen: "127.0.0.1:0", Registry: a.metrics.Registry, Source: eng, Alerts: manager})
	get := func(path string) string {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))
		return rec.Body.String()
	}

	mu.Lock()
	bodies := notifyBodies.String()
	mu.Unlock()
	outputs := map[string]string{
		"logs":                logs.String(),
		"stdout":              stdout.String(),
		"/metrics":            get("/metrics"),
		"/status":             get("/status"),
		"/readyz":             get("/readyz"),
		"notification bodies": bodies,
	}
	// Preconditions: the scenario really exercised the failure paths.
	for _, want := range []string{"notification failed", "dead man", "eth_chainId"} {
		if !strings.Contains(outputs["logs"], want) {
			t.Errorf("precondition: logs do not mention %q:\n%s", want, outputs["logs"])
		}
	}
	if !strings.Contains(outputs["stdout"], "FW-STALE-001") || !strings.Contains(bodies, "FW-STALE-001") {
		t.Errorf("precondition: the stale alert was not sent")
	}
	for out, text := range outputs {
		for name, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Errorf("%s leaked into %s", name, out)
			}
		}
	}
}
