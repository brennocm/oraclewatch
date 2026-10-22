# Contributing

Thanks for considering a contribution.

## Ground rules

- **Read-only.** oraclewatch only uses `eth_call`, `eth_blockNumber`, `eth_chainId` and
  `eth_getBlockByNumber`. Changes that send transactions or handle private keys will not
  be accepted.
- **No `float64` in alert decisions.** Use `math/big` (`big.Int` for raw values,
  `big.Rat` for ratios). `float64` is only allowed when exporting metrics. See
  [ADR 0003](docs/adr/0003-exact-arithmetic-with-math-big.md).
- **No addresses from memory.** Feed addresses come from the official Chainlink data
  files and are verified on-chain; see [docs/research.md](docs/research.md) §4.
- **Secrets never leave the process.** Do not log, export or return RPC URLs, webhook
  URLs or tokens, including inside wrapped errors. Use `internal/redact`.

## Development

Requirements: Go (version in `go.mod`), `golangci-lint`, and optionally `solc` (pinned in
the `Makefile`) to rebuild the test contracts.

```sh
make build            # ./bin/oraclewatch
make test-race        # unit tests with the race detector
make test-integration # simulated-backend tests
make lint vuln
```

## Adding a check

1. Specify it in [docs/research.md](docs/research.md) §8 (ID, logic, defaults, rationale).
2. Implement it in `internal/checks/<name>.go` as a pure function of its input.
3. Add table-driven tests that cover the exact threshold edges, plus fuzz tests for any
   arithmetic.
4. Document it in `docs/checks/<ID>.md`.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for details.

## Commits

[Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `test:`,
`docs:`, `ci:`, `build:`, `refactor:`, `chore:`). Keep commits small; each must build and
pass the tests.
