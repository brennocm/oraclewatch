// Package metrics exposes oraclewatch state as Prometheus metrics.
//
// Cardinality is bounded by construction: every label value is a configured
// name (network, feed, endpoint, reference source, pair), a check ID, or a
// small enum. No address, URL, error message or free text is ever a label.
//
// float64 appears here and only here: Prometheus samples are floats. These
// values are for dashboards; no alert decision reads them.
package metrics

import (
	"errors"
	"math/big"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/reference"
)

const ns = "oraclewatch"

// Metrics holds every collector and implements the observer interfaces of the
// engine, chain, reference and alert packages.
type Metrics struct {
	Registry *prometheus.Registry

	feedAnswer    *prometheus.GaugeVec
	feedAge       *prometheus.GaugeVec
	feedUpdatedAt *prometheus.GaugeVec
	feedRound     *prometheus.GaugeVec
	deviation     *prometheus.GaugeVec
	feedReference *prometheus.GaugeVec
	feedHeartbeat *prometheus.GaugeVec
	checkStatus   *prometheus.GaugeVec
	sequencerUp   *prometheus.GaugeVec
	rpcRequests   *prometheus.CounterVec
	rpcLatency    *prometheus.HistogramVec
	rpcHeight     *prometheus.GaugeVec
	refPrice      *prometheus.GaugeVec
	refErrors     *prometheus.CounterVec
	cycleDuration *prometheus.HistogramVec
	cyclesTotal   *prometheus.CounterVec
	cyclesSkipped *prometheus.CounterVec
	alertsSent    *prometheus.CounterVec
	buildInfo     *prometheus.GaugeVec
}

// New registers every metric on a fresh registry (plus Go and process
// collectors).
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		feedAnswer: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_answer",
			Help: "Latest feed answer normalised by decimals (display only)."}, []string{"network", "feed"}),
		feedAge: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_age_seconds",
			Help: "Pinned block time minus updatedAt."}, []string{"network", "feed"}),
		feedUpdatedAt: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_updated_at_seconds",
			Help: "updatedAt of the latest round (unix seconds)."}, []string{"network", "feed"}),
		feedRound: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_aggregator_round",
			Help: "Aggregator round ID (low 64 bits of the proxy round ID)."}, []string{"network", "feed"}),
		deviation: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_deviation_ratio",
			Help: "|oracle - reference| / reference, when the reference is trusted."}, []string{"network", "feed"}),
		feedReference: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_reference_price",
			Help: "Median market reference used for the feed's deviation check (display only)."}, []string{"network", "feed"}),
		feedHeartbeat: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "feed_heartbeat_seconds",
			Help: "Configured heartbeat of the feed."}, []string{"network", "feed"}),
		checkStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "check_status",
			Help: "Latest check status: 0 ok, 1 info, 2 unknown, 3 warning, 4 critical."}, []string{"check", "network", "feed"}),
		sequencerUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "sequencer_up",
			Help: "1 if the L2 sequencer uptime feed reports up, 0 if down."}, []string{"network"}),
		rpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: "rpc_requests_total",
			Help: "JSON-RPC requests by endpoint name, method and result."}, []string{"network", "endpoint", "method", "result"}),
		rpcLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: ns, Name: "rpc_latency_seconds",
			Help: "JSON-RPC request latency.", Buckets: []float64{.05, .1, .25, .5, 1, 2, 5, 10}}, []string{"network", "endpoint", "method"}),
		rpcHeight: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "rpc_block_height",
			Help: "Latest block number reported by each endpoint."}, []string{"network", "endpoint"}),
		refPrice: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "reference_price",
			Help: "Mid price by reference source (display only)."}, []string{"source", "pair"}),
		refErrors: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: "reference_errors_total",
			Help: "Failed reference requests by source and reason."}, []string{"source", "reason"}),
		cycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: ns, Name: "cycle_duration_seconds",
			Help: "Duration of a monitoring cycle.", Buckets: []float64{.25, .5, 1, 2, 5, 10, 30, 60}}, []string{"network"}),
		cyclesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: "cycles_total",
			Help: "Completed cycles by result (ok: at least one feed read)."}, []string{"network", "result"}),
		cyclesSkipped: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: "cycles_skipped_total",
			Help: "Ticks dropped because the previous cycle was still running."}, []string{"network"}),
		alertsSent: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: "alerts_sent_total",
			Help: "Notifications delivered by channel, severity and result."}, []string{"channel", "severity", "result"}),
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "build_info",
			Help: "Build information; always 1."}, []string{"version", "commit"}),
	}
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.feedAnswer, m.feedAge, m.feedUpdatedAt, m.feedRound, m.deviation, m.feedReference, m.feedHeartbeat, m.checkStatus, m.sequencerUp,
		m.rpcRequests, m.rpcLatency, m.rpcHeight, m.refPrice, m.refErrors,
		m.cycleDuration, m.cyclesTotal, m.cyclesSkipped, m.alertsSent, m.buildInfo)
	return m
}

// SetBuildInfo records the version.
func (m *Metrics) SetBuildInfo(version, commit string) {
	m.buildInfo.WithLabelValues(version, commit).Set(1)
}

func ratFloat(r *big.Rat) float64 {
	f, _ := r.Float64() // display only; precision loss is acceptable here
	return f
}

// ObserveFeed implements engine.Recorder.
func (m *Metrics) ObserveFeed(network, feed string, r *feeds.Reading, age time.Duration) {
	m.feedAnswer.WithLabelValues(network, feed).Set(ratFloat(r.Price))
	m.feedAge.WithLabelValues(network, feed).Set(age.Seconds())
	f, _ := new(big.Float).SetInt(r.Round.UpdatedAt).Float64()
	m.feedUpdatedAt.WithLabelValues(network, feed).Set(f)
	m.feedRound.WithLabelValues(network, feed).Set(float64(r.Round.AggregatorRoundID()))
}

// ObserveDeviation implements engine.Recorder.
func (m *Metrics) ObserveDeviation(network, feed string, dev, reference *big.Rat) {
	m.deviation.WithLabelValues(network, feed).Set(ratFloat(dev))
	m.feedReference.WithLabelValues(network, feed).Set(ratFloat(reference))
}

// SetHeartbeat records a feed's configured heartbeat.
func (m *Metrics) SetHeartbeat(network, feed string, hb time.Duration) {
	m.feedHeartbeat.WithLabelValues(network, feed).Set(hb.Seconds())
}

// ObserveCheck implements engine.Recorder.
func (m *Metrics) ObserveCheck(r checks.Result) {
	m.checkStatus.WithLabelValues(r.CheckID, r.Network, r.Feed).Set(float64(r.Status))
}

// ObserveSequencer implements engine.Recorder.
func (m *Metrics) ObserveSequencer(network string, up bool) {
	v := 0.0
	if up {
		v = 1
	}
	m.sequencerUp.WithLabelValues(network).Set(v)
}

// ObserveCycle implements engine.Recorder.
func (m *Metrics) ObserveCycle(network string, d time.Duration, ok bool) {
	m.cycleDuration.WithLabelValues(network).Observe(d.Seconds())
	result := "ok"
	if !ok {
		result = "failed"
	}
	m.cyclesTotal.WithLabelValues(network, result).Inc()
}

// ObserveSkipped implements engine.Recorder.
func (m *Metrics) ObserveSkipped(network string, n int) {
	m.cyclesSkipped.WithLabelValues(network).Add(float64(n))
}

// ObserveHead implements engine.Recorder.
func (m *Metrics) ObserveHead(network, endpoint string, number uint64) {
	m.rpcHeight.WithLabelValues(network, endpoint).Set(float64(number))
}

// ObserveRPC implements chain.Observer.
func (m *Metrics) ObserveRPC(network, endpoint, method, result string, d time.Duration) {
	m.rpcRequests.WithLabelValues(network, endpoint, method, result).Inc()
	m.rpcLatency.WithLabelValues(network, endpoint, method).Observe(d.Seconds())
}

// ObserveQuote implements reference.Observer.
func (m *Metrics) ObserveQuote(source, pair string, q *reference.Quote, err error) {
	if err != nil {
		m.refErrors.WithLabelValues(source, refReason(err)).Inc()
		return
	}
	m.refPrice.WithLabelValues(source, pair).Set(ratFloat(q.Price))
}

// refReason maps an error to a fixed label value.
func refReason(err error) string {
	switch {
	case errors.Is(err, reference.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, reference.ErrUnsupportedPair):
		return "unsupported_pair"
	case errors.Is(err, reference.ErrInvalidQuote):
		return "invalid_quote"
	}
	return "request_failed"
}

// ObserveSent implements alert.SentObserver.
func (m *Metrics) ObserveSent(channel string, severity checks.Status, err error) {
	result := "ok"
	if err != nil {
		result = "failed"
	}
	m.alertsSent.WithLabelValues(channel, severity.String(), result).Inc()
}
