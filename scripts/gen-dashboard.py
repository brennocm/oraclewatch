#!/usr/bin/env python3
"""Generates deploy/grafana/dashboards/oraclewatch.json.

Kept as code so the dashboard is reviewable and reproducible:
    python3 scripts/gen-dashboard.py > deploy/grafana/dashboards/oraclewatch.json
"""
import json

DS = {"type": "prometheus", "uid": "prometheus"}
SEL = 'network=~"$network",feed=~"$feed"'
_id = 0


def pid():
    global _id
    _id += 1
    return _id


def target(expr, legend="", instant=False):
    t = {"datasource": DS, "expr": expr, "legendFormat": legend, "refId": chr(64 + pid() % 26 or 1)}
    if instant:
        t["instant"], t["range"] = True, False
    return t


STATUS_MAP = [{
    "type": "value",
    "options": {
        "0": {"text": "OK", "color": "green"},
        "1": {"text": "INFO", "color": "blue"},
        "2": {"text": "UNKNOWN", "color": "gray"},
        "3": {"text": "WARNING", "color": "yellow"},
        "4": {"text": "CRITICAL", "color": "red"},
    },
}]


def row(title, y):
    return {"type": "row", "title": title, "id": pid(), "collapsed": False, "gridPos": {"x": 0, "y": y, "w": 24, "h": 1}, "panels": []}


def timeseries(title, targets, x, y, w=12, h=8, unit="short", desc="", thresholds=None):
    p = {
        "type": "timeseries", "title": title, "id": pid(), "datasource": DS, "description": desc,
        "gridPos": {"x": x, "y": y, "w": w, "h": h}, "targets": targets,
        "fieldConfig": {"defaults": {"unit": unit, "custom": {"lineWidth": 1, "fillOpacity": 0, "showPoints": "never"}}, "overrides": []},
        "options": {"legend": {"displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"]}, "tooltip": {"mode": "multi"}},
    }
    if thresholds:
        p["fieldConfig"]["defaults"]["thresholds"] = {"mode": "absolute", "steps": thresholds}
        p["fieldConfig"]["defaults"]["custom"]["thresholdsStyle"] = {"mode": "line"}
    return p


def stat(title, targets, x, y, w=6, h=4, mappings=None, desc="", unit="short"):
    return {
        "type": "stat", "title": title, "id": pid(), "datasource": DS, "description": desc,
        "gridPos": {"x": x, "y": y, "w": w, "h": h}, "targets": targets,
        "fieldConfig": {"defaults": {"unit": unit, "mappings": mappings or [], "color": {"mode": "thresholds"},
                                     "thresholds": {"mode": "absolute", "steps": [{"color": "green", "value": None}]}}, "overrides": []},
        "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "background", "graphMode": "none", "textMode": "value_and_name"},
    }


panels = []
y = 0
panels.append(row("Overview", y)); y += 1
panels.append(stat("Worst check status by network", [target('max by (network) (oraclewatch_check_status{network=~"$network"})', "{{network}}", True)],
                   0, y, 12, 4, STATUS_MAP, "Highest status of any check on the network (critical > warning > unknown > info > ok)."))
panels.append(stat("L2 sequencer", [target('oraclewatch_sequencer_up{network=~"$network"}', "{{network}}", True)], 12, y, 6, 4,
                   [{"type": "value", "options": {"0": {"text": "DOWN", "color": "red"}, "1": {"text": "UP", "color": "green"}}}],
                   "From the L2 sequencer uptime feed (answer 0 = up)."))
alerts_stat = stat("Alerts sent (24h)", [target('sum(increase(oraclewatch_alerts_sent_total{result="ok"}[24h])) or vector(0)', "alerts", True)], 18, y, 6, 4)
alerts_stat["fieldConfig"]["defaults"]["decimals"] = 0
panels.append(alerts_stat)
y += 4
panels.append({
    "type": "table", "title": "Checks not OK", "id": pid(), "datasource": DS,
    "gridPos": {"x": 0, "y": y, "w": 24, "h": 7},
    "targets": [target(f'oraclewatch_check_status{{network=~"$network"}} >= 2', "", True)],
    "transformations": [{"id": "labelsToFields", "options": {}}, {"id": "organize", "options": {"excludeByName": {"Time": True, "__name__": True, "instance": True, "job": True}}}],
    "fieldConfig": {"defaults": {"mappings": STATUS_MAP}, "overrides": [{"matcher": {"id": "byName", "options": "Value"}, "properties": [{"id": "displayName", "value": "status"}, {"id": "custom.cellOptions", "value": {"type": "color-background"}}]}]},
})
y += 7

panels.append(row("Feeds", y)); y += 1
panels.append(timeseries("Feed age vs heartbeat (ratio)", [target(f"oraclewatch_feed_age_seconds{{{SEL}}} / on(network, feed) oraclewatch_feed_heartbeat_seconds{{{SEL}}}", "{{network}} {{feed}}")],
                         0, y, 12, 8, "percentunit", "age / heartbeat. FW-STALE-001 warns above 1 + tolerance (default 110%, or heartbeat + 60s).",
                         [{"color": "green", "value": None}, {"color": "yellow", "value": 1.1}]))
panels.append(timeseries("Feed age", [target(f"oraclewatch_feed_age_seconds{{{SEL}}}", "{{network}} {{feed}}")], 12, y, 12, 8, "s"))
y += 8
panels.append(timeseries("Oracle vs market reference", [
    target(f"oraclewatch_feed_answer{{{SEL}}}", "oracle {{network}} {{feed}}"),
    target(f"oraclewatch_feed_reference_price{{{SEL}}}", "reference {{network}} {{feed}}"),
], 0, y, 12, 8, "currencyUSD", "Normalised answer and the median of USD exchange mids used by FW-DEV-001."))
panels.append(timeseries("Deviation from reference", [target(f"oraclewatch_feed_deviation_ratio{{{SEL}}}", "{{network}} {{feed}}")], 12, y, 12, 8, "percentunit",
                         "|oracle - reference| / reference. Default warning above 2%, critical above 5%.",
                         [{"color": "green", "value": None}, {"color": "yellow", "value": 0.02}, {"color": "red", "value": 0.05}]))
y += 8
panels.append(timeseries("Reference sources relative to their pair mean", [target("oraclewatch_reference_price / on(pair) group_left avg by (pair) (oraclewatch_reference_price)", "{{source}} {{pair}}")], 0, y, 12, 8, "percentunit",
                         "Each exchange's mid divided by the mean of all sources for the pair. Spread beyond max_source_spread (1%) makes the reference untrusted (FW-REF-001)."))
panels.append(timeseries("Reference errors", [target("sum by (source, reason) (rate(oraclewatch_reference_errors_total[5m]))", "{{source}} {{reason}}")], 12, y, 12, 8, "reqps"))
y += 8

panels.append(row("RPC endpoints", y)); y += 1
panels.append(timeseries("Latency p95 by endpoint", [target('histogram_quantile(0.95, sum by (le, network, endpoint) (rate(oraclewatch_rpc_latency_seconds_bucket{network=~"$network"}[5m])))', "{{network}} {{endpoint}}")], 0, y, 8, 8, "s"))
panels.append(timeseries("Errors by endpoint", [target('sum by (network, endpoint, result) (rate(oraclewatch_rpc_requests_total{network=~"$network",result!="ok"}[5m]))', "{{network}} {{endpoint}} {{result}}")], 8, y, 8, 8, "reqps"))
panels.append(timeseries("Block height lag behind best endpoint", [target('max by (network) (oraclewatch_rpc_block_height{network=~"$network"}) - on(network) group_right oraclewatch_rpc_block_height{network=~"$network"}', "{{network}} {{endpoint}}")], 16, y, 8, 8, "short",
                         "Blocks behind the most advanced endpoint of the same network."))
y += 8

panels.append(row("Monitor", y)); y += 1
panels.append(timeseries("Cycle duration p95", [target('histogram_quantile(0.95, sum by (le, network) (rate(oraclewatch_cycle_duration_seconds_bucket{network=~"$network"}[15m])))', "{{network}}")], 0, y, 8, 8, "s"))
panels.append(timeseries("Skipped cycles", [target('increase(oraclewatch_cycles_skipped_total{network=~"$network"}[15m])', "{{network}}")], 8, y, 8, 8))
panels.append(timeseries("Alerts sent", [target("sum by (channel, severity, result) (increase(oraclewatch_alerts_sent_total[1h]))", "{{channel}} {{severity}} {{result}}")], 16, y, 8, 8))

dashboard = {
    "uid": "oraclewatch",
    "title": "oraclewatch",
    "description": "Consumer-side monitoring of Chainlink Data Feeds. Not affiliated with Chainlink Labs.",
    "tags": ["oraclewatch", "oracles"],
    "timezone": "utc",
    "schemaVersion": 39,
    "version": 1,
    "editable": False,
    "refresh": "1m",
    "time": {"from": "now-6h", "to": "now"},
    "templating": {"list": [
        {"name": "network", "label": "Network", "type": "query", "datasource": DS, "query": "label_values(oraclewatch_feed_answer, network)",
         "refresh": 2, "includeAll": True, "multi": True, "current": {"text": "All", "value": "$__all"}},
        # Default to one pair: prices of different assets share no useful axis.
        {"name": "feed", "label": "Feed", "type": "query", "datasource": DS, "query": 'label_values(oraclewatch_feed_answer{network=~"$network"}, feed)',
         "refresh": 2, "includeAll": True, "multi": True, "current": {"text": ["ETH/USD"], "value": ["ETH/USD"]}},
    ]},
    "panels": panels,
}
print(json.dumps(dashboard, indent=2))
