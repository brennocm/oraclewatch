#!/usr/bin/env bash
# Recompiles the Solidity test doubles with a pinned solc and regenerates their
# Go bindings. The compiled ABI and bytecode are committed so that tests do not
# need solc.
#
#   SOLC=solc-0.8.37 scripts/build-contracts.sh          # compile + bindings
#   scripts/build-contracts.sh --bindings-only           # bindings from committed ABI/bin
set -euo pipefail

BINDINGS_ONLY=false
[[ "${1:-}" == "--bindings-only" ]] && BINDINGS_ONLY=true

SOLC="${SOLC:-solc}"
WANT="0.8.37+commit.f401782d"
SRC=testdata/contracts/OracleMocks.sol
OUT=testdata/contracts/build
PKG_DIR=internal/feeds/feedstest

if ! $BINDINGS_ONLY; then
  if ! "$SOLC" --version | grep -q "$WANT"; then
    echo "solc $WANT is required (got: $("$SOLC" --version | tail -n1))" >&2
    exit 1
  fi
  mkdir -p "$OUT"
  "$SOLC" --optimize --optimize-runs 200 --evm-version cancun --metadata-hash none \
    --combined-json abi,bin --pretty-json "$SRC" > "$OUT/OracleMocks.json"
fi

# One package per contract: abigen v2 names output structs after the function
# (LatestRoundDataOutput), which would collide between the two mocks.
for c in MockAggregator MockProxy; do
  pkg=$(echo "$c" | tr '[:upper:]' '[:lower:]')
  mkdir -p "$PKG_DIR/$pkg"
  jq --arg c "$SRC:$c" '.contracts[$c].abi' "$OUT/OracleMocks.json" > "$OUT/$c.abi"
  jq -r --arg c "$SRC:$c" '.contracts[$c].bin' "$OUT/OracleMocks.json" > "$OUT/$c.bin"
  go tool abigen --v2 --abi "$OUT/$c.abi" --bin "$OUT/$c.bin" --pkg "$pkg" --type "$c" --out "$PKG_DIR/$pkg/$pkg.go"
  # Test-only: compiled with the integration tag, like the deploy helper.
  printf '//go:build integration\n\n%s\n' "$(cat "$PKG_DIR/$pkg/$pkg.go")" > "$PKG_DIR/$pkg/$pkg.go"
done
echo "built $OUT and $PKG_DIR/{mockaggregator,mockproxy}"
