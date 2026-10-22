# ABI provenance

JSON cannot carry comments, so the origin of each ABI file is recorded here. The files
are embedded into the binary (`go:embed`) and decoded with go-ethereum's `accounts/abi`
package; see `../abi.go` and ADR 0004 for why generated `bind/v2` bindings are not used
in production code.

Only view functions are included.

| File | Contents | Source |
|---|---|---|
| `AggregatorV3Interface.abi.json` | Full `AggregatorV3Interface`: `decimals`, `description`, `version`, `getRoundData`, `latestRoundData`. | npm `@chainlink/contracts@1.5.0`, file `abi/v0.8/shared/AggregatorV3Interface.abi.json`. Tarball integrity `sha512-1fGJwjvivqAxvVOTqZUEXGR54CATtg0vjcXgSIk4Cfoad2nUhSG/qaWHXjLg1CkNTeOoteoxGQcpP/HiA5HsUA==` (matches the registry). Reformatted with `jq`. |
| `AggregatorProxy.abi.json` | Subset of `EACAggregatorProxy`: `aggregator()`, `phaseId()`, `phaseAggregators(uint16)`. | Verified ABI of the Ethereum ETH/USD proxy `0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419` (contract name `EACAggregatorProxy`) from Sourcify: `https://sourcify.dev/server/v2/contract/1/0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419?fields=abi`. The `@chainlink/contracts@1.5.0` package does not ship this ABI. |
| `AggregatorBounds.abi.json` | Subset of the aggregator: `minAnswer()`, `maxAnswer()`, `typeAndVersion()`. | Verified ABI of the Base ETH/USD `DualAggregator` `0x05c84a58FE042275b37db038bAAcD15F410c7bB0` from Sourcify: `https://sourcify.dev/server/v2/contract/8453/0x05c84a58FE042275b37db038bAAcD15F410c7bB0?fields=abi`. |

## Note on `minAnswer` / `maxAnswer` types

`DualAggregator` declares them as `int256`; `AccessControlledOCR2Aggregator` (for example
`0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5`) declares them as `int192`. Both are returned
as one 32-byte ABI word with the value sign-extended, so decoding as `int256` reads either
correctly. Every `int192` value is a valid `int256`. The binding uses `int256` for that
reason.

All three files were retrieved on 2026-10-22.
