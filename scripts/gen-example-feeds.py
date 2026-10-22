#!/usr/bin/env python3
"""Print the networks/feeds section of configs/example.yaml from the verified
lists in configs/feeds/*.yaml, so addresses are never copied by hand.
Usage: python3 scripts/gen-example-feeds.py configs/feeds > section.yaml
Requires PyYAML only if available; falls back to a minimal line parser."""
import sys, os, re

def parse(path):
    # The feed files are generated with a fixed, flat layout; a tiny parser
    # avoids a PyYAML dependency.
    net = {"feeds": []}
    cur = None
    section = None
    for line in open(path):
        if line.startswith("#") or not line.strip():
            continue
        m = re.match(r"^(\w+):\s*(.*)$", line)
        if m:
            section = m[1]
            if m[2]:
                net[m[1]] = m[2].strip().strip('"')
            continue
        m = re.match(r"^  - name: (.*)$", line)
        if m:
            cur = {"name": m[1].strip()}
            net["feeds"].append(cur)
            continue
        m = re.match(r"^\s+(\w+): (.*)$", line)
        if m:
            target = cur if section == "feeds" else net.setdefault(section, {})
            target.setdefault(m[1], m[2].strip().strip('"'))
    return net

order = ["ethereum", "arbitrum", "base", "optimism"]
env = {"ethereum": "ETH", "arbitrum": "ARB", "base": "BASE", "optimism": "OP"}
explorer = {"ethereum": "https://etherscan.io", "arbitrum": "https://arbiscan.io",
            "base": "https://basescan.org", "optimism": "https://optimistic.etherscan.io"}
d = sys.argv[1]
nets = {n: parse(os.path.join(d, n + ".yaml")) for n in order}
print("networks:")
for n in order:
    x = nets[n]
    l2 = "sequencer_uptime_feed" in x
    print(f"  - name: {n}")
    print(f"    chain_id: {x['chain_id']}")
    print(f"    l2: {'true' if l2 else 'false'}")
    print(f"    rpc_endpoints:")
    print(f"      - name: primary")
    print(f"        url: ${{{env[n]}_RPC_URL_PRIMARY}}")
    print(f"      - name: secondary")
    print(f"        url: ${{{env[n]}_RPC_URL_SECONDARY}}")
    print(f"    poll_interval: 60s")
    print(f"    confirmations: {10 if l2 else 2}")
    print(f"    explorer_url: {explorer[n]}")
    if l2:
        print(f"    sequencer:")
        print(f"      proxy: \"{x['sequencer_uptime_feed']['proxy']}\"  # {x['sequencer_uptime_feed']['description']}")
        print(f"      grace_period: 1h")
print()
print("feeds:")
for n in order:
    print(f"  # --- {n}: generated from configs/feeds/{n}.yaml ---")
    for f in nets[n]["feeds"]:
        pair = f["name"].replace("/", "-")
        print(f"  - name: {f['name']}")
        print(f"    network: {n}")
        print(f"    proxy: \"{f['proxy']}\"")
        print(f"    expected_decimals: {f['decimals']}")
        print(f"    expected_description: \"{f['description']}\"")
        print(f"    heartbeat: {f['heartbeat']}")
        print(f"    deviation_threshold: {f['deviation_threshold']}")
        print(f"    reference:")
        print(f"      pair: {pair}")
        if pair == "USDC-USD":
            print(f"      sources: [kraken, bitstamp, gemini]  # Coinbase has no USDC-USD market")
