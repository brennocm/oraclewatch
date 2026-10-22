package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/alert/notify"
	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/engine"
	"github.com/brennocm/oraclewatch/internal/metrics"
	"github.com/brennocm/oraclewatch/internal/redact"
	"github.com/brennocm/oraclewatch/internal/reference"
)

// app holds everything built from a configuration.
type app struct {
	cfg      *config.Config
	redactor *redact.Redactor
	logger   *slog.Logger
	metrics  *metrics.Metrics
	networks []*chain.Network
	ref      *reference.Aggregator
}

// loadApp loads the config and registers every secret with the redactor
// before anything is logged.
func loadApp(ctx context.Context, path string, logOut io.Writer, withMetrics bool) (*app, error) {
	cfg, err := config.Load(path, os.LookupEnv)
	if err != nil {
		return nil, err
	}
	a := &app{cfg: cfg, redactor: &redact.Redactor{}}
	for _, s := range cfg.Secrets() {
		a.redactor.Add(s)
	}
	a.logger = newLogger(cfg.Log, logOut, a.redactor)
	for _, w := range cfg.Warnings {
		a.logger.Warn("config", "warning", w)
	}

	var rpcObs chain.Observer
	var refObs reference.Observer
	if withMetrics {
		a.metrics = metrics.New()
		rpcObs, refObs = a.metrics, a.metrics
	}
	for i := range cfg.Networks {
		n, err := chain.NewNetworkFromConfig(ctx, &cfg.Networks[i], chain.BuildOptions{Observer: rpcObs, Redactor: a.redactor})
		if err != nil {
			return nil, err
		}
		a.networks = append(a.networks, n)
	}

	httpClient := &http.Client{Timeout: cfg.Reference.Timeout.Duration}
	var sources []reference.Source
	for _, name := range usedSources(cfg) {
		s, err := reference.NewSource(name, httpClient, nil)
		if err != nil {
			return nil, err
		}
		sources = append(sources, s)
	}
	a.ref = reference.NewAggregator(sources, reference.Options{
		MinSources: cfg.Reference.MinSources,
		MaxSpread:  cfg.Reference.MaxSourceSpread.Rat(),
		MaxAge:     cfg.Reference.MaxQuoteAge.Duration,
		Timeout:    cfg.Reference.Timeout.Duration,
		CacheTTL:   cfg.Reference.CacheTTL.Duration,
		Observer:   refObs,
		Redactor:   a.redactor,
	})
	return a, nil
}

// usedSources returns every reference source named globally or by a feed.
func usedSources(cfg *config.Config) []string {
	seen := map[string]bool{}
	var out []string
	add := func(names []string) {
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	add(cfg.Reference.Sources)
	for _, f := range cfg.Feeds {
		if f.Reference != nil {
			add(f.Reference.Sources)
		}
	}
	return out
}

func newLogger(c config.Log, w io.Writer, r *redact.Redactor) *slog.Logger {
	level := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}[c.Level]
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if c.Format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(redact.NewHandler(h, r))
}

func (a *app) engine(alerts *alert.Manager, d *alert.Dispatcher, heartbeat func(context.Context) error) (*engine.Engine, error) {
	var rec engine.Recorder
	if a.metrics != nil {
		rec = a.metrics
	}
	return engine.New(engine.Options{
		Config: a.cfg, Networks: a.networks, Reference: a.ref, Alerts: alerts, Dispatcher: d,
		Logger: a.logger, Recorder: rec, Heartbeat: heartbeat,
	})
}

// notifiers builds the configured alert channels.
func (a *app) notifiers(stdout io.Writer) []alert.Notifier {
	var out []alert.Notifier
	al := a.cfg.Alerting
	if al.Stdout {
		out = append(out, notify.NewStdout(stdout))
	}
	if al.Discord != nil && !al.Discord.WebhookURL.IsZero() {
		out = append(out, notify.NewDiscord(al.Discord.WebhookURL.Value(), nil, a.redactor))
	}
	if al.Telegram != nil && !al.Telegram.BotToken.IsZero() {
		out = append(out, notify.NewTelegram("", al.Telegram.BotToken.Value(), al.Telegram.ChatID.Value(), nil, a.redactor))
	}
	return out
}

// link returns an explorer URL for a result's feed proxy.
func (a *app) link(r checks.Result) string {
	n := a.cfg.Network(r.Network)
	if n == nil || n.ExplorerURL == "" {
		return ""
	}
	for _, f := range a.cfg.Feeds {
		if f.Network == r.Network && f.Name == r.Feed {
			return n.ExplorerURL + "/address/" + f.Proxy.Hex()
		}
	}
	if n.Sequencer != nil && (r.CheckID == "FW-SEQ-001" || r.CheckID == "FW-SEQ-002") {
		return n.ExplorerURL + "/address/" + n.Sequencer.Proxy.Hex()
	}
	return ""
}

// deadman returns the dead man's switch ping, or nil if not configured.
func (a *app) deadman() func(context.Context) error {
	u := a.cfg.SelfHealth.DeadmanURL
	if u.IsZero() {
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.Value(), nil)
		if err != nil {
			return a.redactor.Error(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return a.redactor.Error(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("dead man's switch returned HTTP %d", resp.StatusCode)
		}
		return nil
	}
}
