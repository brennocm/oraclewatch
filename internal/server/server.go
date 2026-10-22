// Package server serves /metrics, /healthz, /readyz and /status.
//
// It listens on 127.0.0.1 by default; exposing it is the operator's decision.
// No endpoint returns configuration or secrets: /status contains check
// results, which only hold on-chain values, configured names and numbers.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/engine"
)

// Source provides the data served by /readyz and /status.
type Source interface {
	Ready() bool
	Health() []engine.NetworkHealth
	Results() []checks.Result
}

// ActiveAlerts lists firing alerts; *alert.Manager implements it.
type ActiveAlerts interface {
	Active() []alert.Active
}

// Options configure the server.
type Options struct {
	Listen   string
	Registry *prometheus.Registry
	Source   Source
	Alerts   ActiveAlerts // optional
	Logger   *slog.Logger
	Version  string
}

// Server wraps http.Server with the oraclewatch routes.
type Server struct {
	srv *http.Server
	o   Options
}

// New builds the server. Timeouts are set explicitly so that a slow client
// cannot hold connections open (gosec G112).
func New(o Options) *Server {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	mux := http.NewServeMux()
	s := &Server{o: o}
	mux.Handle("GET /metrics", promhttp.HandlerFor(o.Registry, promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(o.Logger.Handler(), slog.LevelError)}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /status", s.status)
	s.srv = &http.Server{
		Addr:              o.Listen,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	return s
}

// Handler returns the routes, for tests.
func (s *Server) Handler() http.Handler { return s.srv.Handler }

// ListenAndServe serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(ln) }()
	s.o.Logger.Info("http server listening", "addr", ln.Addr().String())
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	body := map[string]any{"ready": s.o.Source.Ready(), "networks": s.o.Source.Health()}
	code := http.StatusOK
	if !s.o.Source.Ready() {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, body)
}

type statusResult struct {
	Check    string            `json:"check"`
	Network  string            `json:"network"`
	Feed     string            `json:"feed,omitempty"`
	Status   string            `json:"status"`
	Summary  string            `json:"summary"`
	Details  map[string]string `json:"details,omitempty"`
	Observed time.Time         `json:"observed"`
}

type statusAlert struct {
	Key    string    `json:"key"`
	State  string    `json:"state"`
	Status string    `json:"status"`
	Since  time.Time `json:"since"`
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	results := s.o.Source.Results()
	out := struct {
		Version  string                 `json:"version"`
		Ready    bool                   `json:"ready"`
		Networks []engine.NetworkHealth `json:"networks"`
		Alerts   []statusAlert          `json:"active_alerts"`
		Results  []statusResult         `json:"results"`
	}{Version: s.o.Version, Ready: s.o.Source.Ready(), Networks: s.o.Source.Health(), Alerts: []statusAlert{}, Results: make([]statusResult, 0, len(results))}
	for _, r := range results {
		out.Results = append(out.Results, statusResult{r.CheckID, r.Network, r.Feed, r.Status.String(), r.Summary, r.Details, r.Observed})
	}
	if s.o.Alerts != nil {
		for _, a := range s.o.Alerts.Active() {
			out.Alerts = append(out.Alerts, statusAlert{a.Key, a.State.String(), a.Status.String(), a.Since})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v) // the client may have gone away; nothing to do
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}
